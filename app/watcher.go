package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

var uploadedFiles sync.Map

func MarkUploading(path string)  { uploadedFiles.Store(path, time.Now()) }
func ClearUploading(path string)  { uploadedFiles.Delete(path) }

func isRecentlyUploaded(path string) bool {
	v, ok := uploadedFiles.Load(path)
	if !ok {
		return false
	}
	if time.Since(v.(time.Time)) < 3*time.Second {
		return true
	}
	uploadedFiles.Delete(path)
	return false
}

func StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建文件监听器失败: %w", err)
	}

	err = filepath.Walk(AppConfig.MountPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(AppConfig.MountPath, path)
		if isTrashPath(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if relPath != "." && isHiddenFile(info.Name()) && info.IsDir() {
			return filepath.SkipDir
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				log.Printf("[监听] 添加监听失败: %s, %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("遍历监听目录失败: %w", err)
	}

	log.Println("[监听] 文件系统监听已启动")

	go func() {
		defer watcher.Close()

		debounce := make(map[string]time.Time)
		debounceInterval := 500 * time.Millisecond
		debounceCleanTicker := time.NewTicker(10 * time.Minute)
		defer debounceCleanTicker.Stop()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				relPath, _ := filepath.Rel(AppConfig.MountPath, event.Name)
				if isTrashPath(relPath) {
					continue
				}

				name := filepath.Base(event.Name)
				if isHiddenFile(name) {
					continue
				}

				if last, ok := debounce[event.Name]; ok {
					if time.Since(last) < debounceInterval {
						continue
					}
				}
				debounce[event.Name] = time.Now()

				handleFsEvent(watcher, event)

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[监听] 错误: %v", err)

			case <-debounceCleanTicker.C:
				now := time.Now()
				for k, t := range debounce {
					if now.Sub(t) > 5*time.Minute {
						delete(debounce, k)
					}
				}
			}
		}
	}()

	return nil
}

func handleFsEvent(watcher *fsnotify.Watcher, event fsnotify.Event) {
	name := filepath.Base(event.Name)
	if strings.HasPrefix(name, ".upload") {
		return
	}

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		handleCreate(watcher, event.Name)
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		handleFsRename(event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		handleRemove(event.Name)
	case event.Op&fsnotify.Write == fsnotify.Write:
		handleWrite(event.Name)
	}
}

func handleCreate(watcher *fsnotify.Watcher, path string) {
	if isRecentlyUploaded(path) {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	if info.IsDir() {
		watcher.Add(path)
	}

	rec := &FileRecord{
		UUID:       uuid.New().String(),
		Name:       info.Name(),
		ParentPath: filepath.Dir(path),
		FullPath:   path,
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		Ext:        getExt(info.Name()),
	}

	if err := UpsertFile(rec); err != nil {
		log.Printf("[监听] 创建同步失败: %s, %v", path, err)
	}

	if !info.IsDir() {
		SubmitThumbnailTask(path)
		go func() {
			if md5Val, err := ComputeFileMD5(path); err == nil {
				UpdateFileMD5(path, md5Val)
			}
		}()
	}
}

func handleFsRename(path string) {
	if isRecentlyUploaded(path) {
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		MarkFileTrashed(path)
	}
}

func handleRemove(path string) {
	if isRecentlyUploaded(path) {
		return
	}
	MarkFileTrashed(path)
}

func handleWrite(path string) {
	if isRecentlyUploaded(path) {
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}

	DB.Exec(`UPDATE files SET size=? WHERE full_path=? AND status='active'`, info.Size(), path)

	ext := getExt(filepath.Base(path))
	if imageExts[ext] || videoExts[ext] {
		SubmitThumbnailTask(path)
	}
}
