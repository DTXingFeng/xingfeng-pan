package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	logMaxSize  = 10 * 1024 * 1024 // 单文件最大 10MB
	logMaxFiles = 3                // 保留 3 个旧日志
)

type rotateWriter struct {
	mu       sync.Mutex
	file     *os.File
	size     int64
	logDir   string
	baseName string
}

// InitLogger 初始化日志系统，同时输出到控制台和文件
func InitLogger() {
	logDir := filepath.Join(DataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("[日志] 创建日志目录失败: %v，仅使用控制台输出\n", err)
		return
	}

	w := &rotateWriter{
		logDir:   logDir,
		baseName: "pan.log",
	}

	logPath := filepath.Join(logDir, "pan.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("[日志] 打开日志文件失败: %v，仅使用控制台输出\n", err)
		return
	}

	info, _ := f.Stat()
	w.file = f
	if info != nil {
		w.size = info.Size()
	}

	// 同时输出到控制台和文件
	multi := io.MultiWriter(os.Stdout, w)
	log.SetOutput(multi)
	log.SetFlags(log.Ldate | log.Ltime)

	fmt.Printf("[日志] 日志文件: %s\n", logPath)
}

func (w *rotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return 0, fmt.Errorf("日志文件未打开")
	}

	// 检查是否需要轮转
	if w.size+int64(len(p)) > logMaxSize {
		w.rotate()
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return
}

// rotate 执行日志轮转
func (w *rotateWriter) rotate() {
	w.file.Close()

	// 当前文件重命名为带时间戳的归档文件
	currentPath := filepath.Join(w.logDir, w.baseName)
	archiveName := fmt.Sprintf("pan_%s.log", time.Now().Format("20060102_150405"))
	archivePath := filepath.Join(w.logDir, archiveName)
	os.Rename(currentPath, archivePath)

	// 清理多余的旧日志
	w.cleanOldLogs()

	// 创建新日志文件
	f, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("[日志] 创建新日志文件失败: %v\n", err)
		return
	}
	w.file = f
	w.size = 0
}

// cleanOldLogs 保留最新的 logMaxFiles 个归档文件
func (w *rotateWriter) cleanOldLogs() {
	entries, err := os.ReadDir(w.logDir)
	if err != nil {
		return
	}

	var archives []string
	for _, e := range entries {
		name := e.Name()
		if name != w.baseName && filepath.Ext(name) == ".log" {
			archives = append(archives, name)
		}
	}

	if len(archives) <= logMaxFiles {
		return
	}

	sort.Strings(archives)
	toDelete := archives[:len(archives)-logMaxFiles]
	for _, name := range toDelete {
		os.Remove(filepath.Join(w.logDir, name))
	}
}
