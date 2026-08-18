package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

//go:embed frontend/dist
var frontendFS embed.FS

// RegisterFrontend 注册前端静态文件服务
func RegisterFrontend(router *mux.Router) {
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return
	}

	fileServer := http.FileServer(http.FS(subFS))

	// SPA 前端处理器
	spaHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 跳过 API 和 WebDAV 路径
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/dav") {
			http.NotFound(w, r)
			return
		}

		// 去掉前导斜杠得到相对路径
		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		// 尝试打开静态文件
		f, err := subFS.Open(cleanPath)
		if err == nil {
			f.Close()
			// 文件存在，交给 FileServer 处理
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA 路由 fallback：找不到文件时返回 index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// 匹配所有路径作为前端 fallback（优先级最低，放在最后）
	router.PathPrefix("/").Handler(spaHandler)
}
