package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/webdav"
)

// InitWebDAV 初始化 WebDAV 服务
func InitWebDAV() http.Handler {
	// 创建 WebDAV Handler，根目录映射到挂载目录
	davHandler := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: webdav.Dir(AppConfig.MountPath),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("[WebDAV] %s %s 错误: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	// 包装 Basic Auth 中间件
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic Auth 验证
		user, pass, ok := r.BasicAuth()
		if !ok || user != AppConfig.WebdavUser || !checkWebdavPassword(pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="WebDAV"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		davHandler.ServeHTTP(w, r)
	})
}

// checkWebdavPassword 验证 WebDAV 密码（兼容明文和 bcrypt 哈希）
func checkWebdavPassword(input string) bool {
	stored := AppConfig.WebdavPass
	// 如果配置的密码是 bcrypt 哈希（$2a$ / $2b$ / $2y$ 开头），使用 bcrypt 比对
	if len(stored) > 4 && stored[:4] == "$2a$" || len(stored) > 4 && stored[:4] == "$2b$" || len(stored) > 4 && stored[:4] == "$2y$" {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(input)) == nil
	}
	// 否则明文比对
	return input == stored
}

// HashPassword 生成 bcrypt 密码哈希（备用，当前直接比对明文）
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// sanitizePath 清理路径，防止目录穿越（跨平台）
func sanitizePath(basePath, userPath string) (string, error) {
	// 统一分隔符后清理
	cleanUser := filepath.Clean(string(os.PathSeparator) + userPath)
	cleanPath := filepath.Join(basePath, cleanUser)

	// 统一为绝对路径比较，避免 Windows 大小写和斜杠差异
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", fmt.Errorf("非法路径访问")
	}
	absClean, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("非法路径访问")
	}

	// Windows 路径大小写不敏感比较
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(strings.ToLower(absClean), strings.ToLower(absBase)) {
			return "", fmt.Errorf("非法路径访问")
		}
	} else {
		if !strings.HasPrefix(absClean, absBase) {
			return "", fmt.Errorf("非法路径访问")
		}
	}
	return cleanPath, nil
}

// serveFileRange 支持范围请求的文件服务
func serveFileRange(w http.ResponseWriter, r *http.Request, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "文件打开失败", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "文件信息获取失败", http.StatusInternalServerError)
		return
	}

	fileSize := stat.Size()
	rangeHeader := r.Header.Get("Range")

	if rangeHeader == "" {
		// 无 Range 请求，返回完整文件
		w.Header().Set("Content-Type", detectContentType(filePath))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
		return
	}

	// 解析 Range 头
	// 格式: bytes=start-end 或 bytes=start- 或 bytes=-suffix
	var start, end int64
	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.SplitN(rangeSpec, "-", 2)
	if len(parts) != 2 {
		http.Error(w, "Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	if parts[0] == "" {
		// bytes=-suffix：从末尾取 suffix 字节
		fmt.Sscanf(parts[1], "%d", &end)
		start = fileSize - end
		end = fileSize - 1
	} else {
		fmt.Sscanf(parts[0], "%d", &start)
		if parts[1] == "" {
			// bytes=start-：从 start 到末尾
			end = fileSize - 1
		} else {
			fmt.Sscanf(parts[1], "%d", &end)
		}
	}

	// 默认 end 为文件末尾
	if end >= fileSize {
		end = fileSize - 1
	}

	// 校验范围
	if start < 0 || start >= fileSize || start > end {
		http.Error(w, "Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// 计算内容长度
	contentLength := end - start + 1

	// 发送 206 响应
	w.Header().Set("Content-Type", detectContentType(filePath))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)

	// 定位到起始位置并写入响应
	file.Seek(start, 0)
	buf := make([]byte, 32*1024) // 32KB 缓冲
	remaining := contentLength
	for remaining > 0 {
		toRead := int64(len(buf))
		if remaining < toRead {
			toRead = remaining
		}
		n, err := file.Read(buf[:toRead])
		if err != nil {
			break
		}
		w.Write(buf[:n])
		remaining -= int64(n)
	}
}

// detectContentType 根据文件扩展名检测 Content-Type
func detectContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	contentTypes := map[string]string{
		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mkv":  "video/x-matroska",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".m4v":  "video/mp4",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".flac": "audio/flac",
		".aac":  "audio/aac",
		".ogg":  "audio/ogg",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".bmp":  "image/bmp",
		".pdf":  "application/pdf",
		".txt":  "text/plain; charset=utf-8",
		".html": "text/html; charset=utf-8",
		".json": "application/json",
	}
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}
