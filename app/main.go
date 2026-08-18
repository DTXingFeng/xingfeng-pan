package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var startTime time.Time

func main() {
	startTime = time.Now()
	fmt.Println("============================================")
	fmt.Println("  刑风网盘 v1.0 启动中...")
	fmt.Println("============================================")

	// 1. 初始化配置
	if err := InitConfig(); err != nil {
		log.Fatalf("配置初始化失败: %v", err)
	}

	// 2. 初始化数据目录
	if err := InitDirectories(); err != nil {
		log.Fatalf("目录初始化失败: %v", err)
	}

	// 3. 初始化日志（同时输出到控制台和文件）
	InitLogger()

	// 4. 初始化数据库
	if err := InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer CloseDB()

	// 5. 初始化缩略图 Worker
	InitThumbnailWorker()
	defer StopThumbnailWorker()

	// 6. 全量扫描
	if err := BootScan(); err != nil {
		log.Fatalf("全量扫描失败: %v", err)
	}

	// 7. 启动文件监听
	if err := StartWatcher(); err != nil {
		log.Fatalf("文件监听启动失败: %v", err)
	}

	// 8. 初始化 API 和前端路由
	handler := InitAPI()

	// 9. 挂载 WebDAV
	webdavHandler := InitWebDAV()

	// 组合路由
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/dav" || strings.HasPrefix(path, "/dav/") {
			webdavHandler.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})

	// 10. 启动 HTTP 服务器
	addr := fmt.Sprintf(":%d", AppConfig.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           finalHandler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      600 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// 回收站自动清理
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			TrashAutoClean(30)
		}
	}()

	// 分片上传自动清理（清理超过24小时的未完成上传）
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanStaleChunkUploads(24 * time.Hour)
		}
	}()

	// 缩略图自动补生成：每30分钟检查一次待处理的缩略图
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			generatePendingThumbnails()
		}
	}()

	// 优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("[系统] 正在关闭服务器...")
		server.Close()
	}()

	log.Println("============================================")
	log.Printf("  服务器已启动: http://0.0.0.0%s", addr)
	log.Printf("  WebDAV 地址: http://0.0.0.0%s/dav", addr)
	log.Println("============================================")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务器启动失败: %v", err)
	}

	log.Println("[系统] 服务器已安全关闭")
}
