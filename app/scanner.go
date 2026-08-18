package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func BootScan() error {
	log.Println("[扫描] 开始全量扫描挂载目录...")
	mountPath := AppConfig.MountPath

	if err := ensureTrashDir(); err != nil {
		return fmt.Errorf("创建回收站目录失败: %w", err)
	}

	existingPaths := make(map[string]bool)
	var thumbTasks []string
	count := 0
	failCount := 0
	var batch []*FileRecord
	const batchSize = 500

	thumbedRows, _ := DB.Query(`SELECT full_path FROM files WHERE status='active' AND has_thumbnail >= 1`)
	if thumbedRows != nil {
		for thumbedRows.Next() {
			var p string
			if thumbedRows.Scan(&p) == nil {
				existingPaths[p] = true
			}
		}
		thumbedRows.Close()
	}

	err := filepath.Walk(mountPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			failCount++
			return nil
		}

		if path == mountPath {
			return nil
		}

		relPath, _ := filepath.Rel(mountPath, path)
		if isTrashPath(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := info.Name()
		if isHiddenFile(name) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rec := &FileRecord{
			UUID:       uuid.New().String(),
			Name:       name,
			ParentPath: filepath.Dir(path),
			FullPath:   path,
			IsDir:      info.IsDir(),
			Size:       info.Size(),
			Ext:        getExt(name),
		}

		batch = append(batch, rec)
		count++

		if !info.IsDir() {
			ext := getExt(name)
			if (imageExts[ext] || videoExts[ext]) && existingPaths[path] == false {
				thumbTasks = append(thumbTasks, path)
			}
		}

		existingPaths[path] = true

		// 每积累 batchSize 条记录提交一次事务
		if len(batch) >= batchSize {
			if err := BatchUpsertFiles(batch); err != nil {
				log.Printf("[扫描] 批量写入失败: %v", err)
				failCount += len(batch)
			}
			batch = batch[:0]
		}

		return nil
	})

	// 提交剩余的记录
	if len(batch) > 0 {
		if err := BatchUpsertFiles(batch); err != nil {
			log.Printf("[扫描] 批量写入剩余记录失败: %v", err)
			failCount += len(batch)
		}
	}

	if err != nil {
		return fmt.Errorf("扫描目录失败: %w", err)
	}

	if err := DeleteStaleRecords(existingPaths); err != nil {
		log.Printf("[扫描] 清理过期记录失败: %v", err)
	}

	msg := fmt.Sprintf("[扫描] 全量扫描完成，同步了 %d 个文件/目录", count)
	if failCount > 0 {
		msg += fmt.Sprintf("，%d 个失败", failCount)
	}

	// 无条件启动 Worker，确保运行时提交的任务能被处理
	StartThumbnailWorkers()

	if len(thumbTasks) > 0 {
		for _, t := range thumbTasks {
			SubmitThumbnailTask(t)
		}
		msg += fmt.Sprintf("，%d 个待生成缩略图", len(thumbTasks))
	}
	log.Println(msg)

	return nil
}
