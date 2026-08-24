package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginRateLimit 登录频率限制
type loginAttempt struct {
	count     int
	firstSeen time.Time
	locked    bool
}

var (
	loginLimiter  = make(map[string]*loginAttempt)
	loginLimitMu  sync.Mutex
	loginMaxFails = 5
	loginLockDur  = 10 * time.Minute
	loginCleanDur = 30 * time.Minute
)

func init() {
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			loginLimitMu.Lock()
			now := time.Now()
			for ip, a := range loginLimiter {
				if a.locked && now.Sub(a.firstSeen) > loginLockDur {
					delete(loginLimiter, ip)
				} else if !a.locked && now.Sub(a.firstSeen) > loginCleanDur {
					delete(loginLimiter, ip)
				}
			}
			loginLimitMu.Unlock()
		}
	}()
}

func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	ip = r.Header.Get("X-Real-IP")
	if ip != "" {
		return strings.TrimSpace(ip)
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

// handleLogin 登录接口
// POST /api/auth/login {"token": "xxx", "remember": true}
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if AppConfig.AuthToken == "" {
		respondJSON(w, http.StatusOK, map[string]string{"token": ""})
		return
	}

	clientIP := getClientIP(r)

	loginLimitMu.Lock()
	attempt, exists := loginLimiter[clientIP]
	if !exists {
		attempt = &loginAttempt{firstSeen: time.Now()}
		loginLimiter[clientIP] = attempt
	}
	if attempt.locked {
		remaining := loginLockDur - time.Since(attempt.firstSeen)
		if remaining > 0 {
			loginLimitMu.Unlock()
			log.Printf("[认证] IP %s 已被锁定, 剩余 %v", clientIP, remaining.Truncate(time.Second))
			respondError(w, http.StatusTooManyRequests, "登录尝试次数过多，请 "+formatDuration(remaining)+" 后重试")
			return
		}
		delete(loginLimiter, clientIP)
		attempt = &loginAttempt{firstSeen: time.Now()}
		loginLimiter[clientIP] = attempt
	}
	loginLimitMu.Unlock()

	var body struct {
		Token    string `json:"token"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.Token != AppConfig.AuthToken {
		loginLimitMu.Lock()
		attempt.count++
		remainingAttempts := loginMaxFails - attempt.count
		if attempt.count >= loginMaxFails {
			attempt.locked = true
			attempt.firstSeen = time.Now()
			loginLimitMu.Unlock()
			log.Printf("[认证] IP %s 登录锁定 (失败 %d 次)", clientIP, attempt.count)
			respondError(w, http.StatusTooManyRequests, "登录尝试次数过多，请 "+formatDuration(loginLockDur)+" 后重试")
			return
		}
		loginLimitMu.Unlock()
		log.Printf("[认证] IP %s 登录失败 (剩余 %d 次)", clientIP, remainingAttempts)
		respondError(w, http.StatusUnauthorized, "token 无效")
		return
	}

	loginLimitMu.Lock()
	delete(loginLimiter, clientIP)
	loginLimitMu.Unlock()

	cookie := &http.Cookie{
		Name:     "token",
		Value:    body.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if body.Remember {
		cookie.MaxAge = 86400 * 7 // 记住7天
	}
	http.SetCookie(w, cookie)

	log.Printf("[认证] 登录成功 (来自 %s, 记住: %v)", r.RemoteAddr, body.Remember)
	respondJSON(w, http.StatusOK, map[string]string{"token": body.Token})
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Truncate(time.Second).String()
	}
	if d < time.Hour {
		return d.Truncate(time.Minute).String()
	}
	return d.Truncate(time.Hour).String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// handleLogout 登出接口
// POST /api/auth/logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	log.Printf("[认证] 已登出 (来自 %s)", r.RemoteAddr)
	respondJSON(w, http.StatusOK, nil)
}

// handleAuthCheck 检查认证状态接口
// GET /api/auth/check
func handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	// 如果未启用认证，直接返回已认证
	if AppConfig.AuthToken == "" {
		respondJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
		return
	}

	// 检查 cookie 中的 token
	cookie, err := r.Cookie("token")
	if err != nil || cookie.Value != AppConfig.AuthToken {
		respondJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// authMiddleware 认证中间件，AuthToken 为空时跳过认证
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 未启用认证，直接放行
		if AppConfig.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		// 跳过登录相关接口
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// 跳过 OPTIONS 预检请求
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// 优先从 cookie 读取 token
		token := ""
		if cookie, err := r.Cookie("token"); err == nil {
			token = cookie.Value
		}

		// 兜底从 Authorization header 读取
		if token == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		// 兜底从 URL 查询参数读取（PotPlayer/VLC 等外部播放器无法携带 cookie）
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" || token != AppConfig.AuthToken {
			respondError(w, http.StatusUnauthorized, "未认证")
			return
		}

		next.ServeHTTP(w, r)
	})
}
