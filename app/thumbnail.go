package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"
)

var imageExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "bmp": true, "webp": true, "tiff": true, "tif": true,
}

var videoExts = map[string]bool{
	"mp4": true, "avi": true, "mkv": true, "mov": true, "wmv": true, "flv": true, "webm": true, "m4v": true, "mpeg": true, "mpg": true, "3gp": true,
}

type ThumbnailWorker struct {
	taskChan chan string
	wg       sync.WaitGroup
}

var thumbWorker *ThumbnailWorker
var thumbTotal      atomic.Int64
var thumbSuccess    atomic.Int64
var thumbFailed     atomic.Int64
var thumbPrevTotal  atomic.Int64
var thumbReportStop chan struct{}
var thumbReportOnce sync.Once
var thumbSubmitWg   sync.WaitGroup
var thumbDone       chan struct{}
var thumbCache      sync.Map

func getThumbConcurrency() int {
	n := runtime.NumCPU()
	if n < 2 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

func SubmitThumbnailTask(fullPath string) {
	if thumbWorker == nil {
		return
	}
	thumbTotal.Add(1)
	thumbSubmitWg.Add(1)
	go func() {
		defer func() {
			recover()
			thumbSubmitWg.Done()
		}()
		thumbWorker.taskChan <- fullPath
	}()
	thumbReportOnce.Do(startProgressReporter)
}

func (tw *ThumbnailWorker) worker(id int) {
	defer tw.wg.Done()
	for path := range tw.taskChan {
		time.Sleep(10 * time.Millisecond)
		generateThumbnail(path)
	}
}

func generateThumbnail(fullPath string) {
	ext := strings.ToLower(filepath.Ext(fullPath))
	if len(ext) > 0 {
		ext = ext[1:]
	}

	if !imageExts[ext] && !videoExts[ext] {
		SetThumbnail(fullPath, 1)
		thumbSuccess.Add(1)
		return
	}

	if _, ok := thumbCache.Load(fullPath); ok {
		thumbSuccess.Add(1)
		return
	}

	name := filepath.Base(fullPath)

	var rec *FileRecord
	var err error
	for retry := 0; retry < 3; retry++ {
		rec, err = GetFileByPath(fullPath)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
	}
	if err != nil {
		log.Printf("[缩略图] 失败: 查询数据库找不到 %s, %v", name, err)
		thumbFailed.Add(1)
		return
	}
	if rec.HasThumbnail {
		thumbCache.Store(fullPath, true)
		thumbSuccess.Add(1)
		return
	}

	maxRetries := 2
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var cacheName string
		var genErr error

		if imageExts[ext] {
			cacheName, genErr = generateImageThumbnail(fullPath, rec)
		} else {
			cacheName, genErr = generateVideoThumbnail(fullPath, rec)
		}

		if genErr != nil {
			log.Printf("[缩略图] 生成失败 (第%d次): %s, %v", attempt, name, genErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			log.Printf("[缩略图] 最终失败: %s", name)
			SetThumbnail(fullPath, 2)
			thumbFailed.Add(1)
			return
		}

		if cacheName != "" {
			if err := SetThumbnail(fullPath, 1); err != nil {
				log.Printf("[缩略图] 更新数据库失败: %s, %v", name, err)
				thumbFailed.Add(1)
				return
			}
			thumbCache.Store(fullPath, true)
			log.Printf("[缩略图] 生成成功: %s", name)
		} else {
			log.Printf("[缩略图] 生成返回空: %s", name)
		}
		thumbSuccess.Add(1)
		return
	}
}

func isFileStable(path string) bool {
	info1, err := os.Stat(path)
	if err != nil {
		return false
	}
	time.Sleep(500 * time.Millisecond)
	info2, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info1.Size() == info2.Size() && info1.ModTime() == info2.ModTime()
}

func generateImageThumbnail(fullPath string, rec *FileRecord) (string, error) {
	img, err := imaging.Open(fullPath, imaging.AutoOrientation(true))
	if err != nil {
		return "", fmt.Errorf("打开图片失败: %w", err)
	}

	thumb := imaging.Thumbnail(img, 256, 256, imaging.Lanczos)

	cacheName := rec.UUID + ".jpg"
	cachePath := filepath.Join(CacheDir, cacheName)

	if err := imaging.Save(thumb, cachePath); err != nil {
		return "", fmt.Errorf("保存缩略图失败: %w", err)
	}

	return cacheName, nil
}

func generateVideoThumbnail(fullPath string, rec *FileRecord) (string, error) {
	cacheName := rec.UUID + ".jpg"
	cachePath := filepath.Join(CacheDir, cacheName)

	if _, err := os.Stat(cachePath); err == nil {
		return cacheName, nil
	}

	strategies := []struct {
		name string
		args []string
	}{
		{"seek 2s", []string{"-ss", "00:00:02", "-i", fullPath, "-vframes", "1", "-q:v", "2"}},
		{"no seek", []string{"-i", fullPath, "-vframes", "1", "-q:v", "2"}},
		{"probe deep", []string{"-analyzeduration", "10M", "-probesize", "10M", "-i", fullPath, "-vframes", "1", "-q:v", "2"}},
	}

	for _, s := range strategies {
		os.Remove(cachePath)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, AppConfig.FFmpegPath, append([]string{"-y"}, s.args...)...)
		cmd.Args = append(cmd.Args, cachePath)

		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			lastLines := lines
			if len(lines) > 3 {
				lastLines = lines[len(lines)-3:]
			}
			log.Printf("[缩略图] ffmpeg策略[%s]失败: %s", s.name, strings.Join(lastLines, " | "))
			continue
		}

		if _, err := os.Stat(cachePath); err != nil {
			log.Printf("[缩略图] ffmpeg策略[%s]未生成输出", s.name)
			continue
		}

		return cacheName, nil
	}

	return "", fmt.Errorf("所有策略均失败")
}

func GetThumbnailPath(uuid string) string {
	return filepath.Join(CacheDir, uuid+".jpg")
}

func InitThumbnailWorker() {
	concurrency := getThumbConcurrency()
	thumbWorker = &ThumbnailWorker{
		taskChan: make(chan string, concurrency*8),
	}
	thumbDone = make(chan struct{})
	thumbTotal.Store(0)
	thumbSuccess.Store(0)
	thumbFailed.Store(0)
	log.Printf("[缩略图] Worker Pool 已就绪 (并发: %d)", concurrency)
}

func StartThumbnailWorkers() {
	if thumbWorker == nil {
		return
	}
	concurrency := getThumbConcurrency()
	for i := 0; i < concurrency; i++ {
		thumbWorker.wg.Add(1)
		go thumbWorker.worker(i)
	}
	thumbPrevTotal.Store(thumbTotal.Load())
	thumbReportOnce.Do(startProgressReporter)
}

func startProgressReporter() {
	thumbReportStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		lastDone := int64(0)
		idleTicks := 0
		const maxIdleTicks = 6 // 30秒无新完成则退出

		for {
			select {
			case <-ticker.C:
				done := thumbSuccess.Load() + thumbFailed.Load()
				if done == lastDone && done >= thumbPrevTotal.Load() {
					idleTicks++
					if idleTicks >= maxIdleTicks {
						log.Printf("[缩略图] 完成: 总计 %d, 成功 %d, 失败 %d",
							thumbTotal.Load(), thumbSuccess.Load(), thumbFailed.Load())
						return
					}
				} else {
					idleTicks = 0
				}
				lastDone = done
			case <-thumbReportStop:
				return
			}
		}
	}()
}

func StopThumbnailWorker() {
	if thumbWorker != nil {
		if thumbReportStop != nil {
			close(thumbReportStop)
		}
		close(thumbWorker.taskChan)
	}
}

// generatePendingThumbnails 查询所有 has_thumbnail=0 的图片/视频文件并提交到缩略图队列
func generatePendingThumbnails() {
	if thumbWorker == nil {
		return
	}

	extClauses := make([]string, 0, len(imageExts)+len(videoExts))
	for ext := range imageExts {
		extClauses = append(extClauses, "'"+ext+"'")
	}
	for ext := range videoExts {
		extClauses = append(extClauses, "'"+ext+"'")
	}
	extIn := strings.Join(extClauses, ",")

	query := "SELECT full_path FROM files WHERE has_thumbnail=0 AND is_dir=0 AND status='active' AND ext IN (" + extIn + ")"
	rows, err := DB.Query(query)
	if err != nil {
		log.Printf("[缩略图] 自动补生成查询失败: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var fullPath string
		if rows.Scan(&fullPath) == nil {
			SubmitThumbnailTask(fullPath)
			count++
		}
	}

	if count > 0 {
		StartThumbnailWorkers()
		log.Printf("[缩略图] 自动补生成: %d 个文件已加入队列", count)
	}
}
