package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TrashMove 将文件移入回收站
func TrashMove(fullPath string) error {
	rec, err := GetFileByPath(fullPath)
	if err != nil {
		return fmt.Errorf("文件不存在: %s", fullPath)
	}

	// 物理文件已被外部删除/移动（数据库残留的“幽灵”记录）：
	// 无实体可移入回收站，直接清除记录，否则网页上永远删不掉
	if _, statErr := os.Lstat(fullPath); statErr != nil && os.IsNotExist(statErr) {
		if err := purgeGhostRecord(rec); err != nil {
			return fmt.Errorf("清除失效记录失败: %w", err)
		}
		log.Printf("[回收站] 物理文件已不存在，直接清除记录: %s", fullPath)
		return nil
	}

	if err := ensureTrashDir(); err != nil {
		return fmt.Errorf("创建回收站失败: %w", err)
	}

	trashName := rec.Name
	if rec.Ext != "" {
		trashName = fmt.Sprintf("%s_%s.%s", strings.TrimSuffix(rec.Name, "."+rec.Ext), rec.UUID, rec.Ext)
	} else {
		trashName = fmt.Sprintf("%s_%s", rec.Name, rec.UUID)
	}
	trashPath := filepath.Join(AppConfig.MountPath, ".trash", trashName)

	if err := os.Rename(fullPath, trashPath); err != nil {
		// rename 时源文件恰好被外部删除（竞态）：同样按幽灵记录处理
		if _, statErr := os.Lstat(fullPath); statErr != nil && os.IsNotExist(statErr) {
			if err := purgeGhostRecord(rec); err != nil {
				return fmt.Errorf("清除失效记录失败: %w", err)
			}
			log.Printf("[回收站] 物理文件已不存在，直接清除记录: %s", fullPath)
			return nil
		}
		return fmt.Errorf("移动到回收站失败: %w", err)
	}

	// 更新文件夹自身状态
	if err := MarkFileTrashed(fullPath); err != nil {
		return fmt.Errorf("更新数据库状态失败: %w", err)
	}

	// 如果是文件夹，将所有子文件也标记为 trashed
	if rec.IsDir {
		prefix := fullPath + string(filepath.Separator)
		_, err := DB.Exec(`UPDATE files SET status='trashed', deleted_at=datetime('now') WHERE full_path LIKE ? AND status='active'`, prefix+"%")
		if err != nil {
			log.Printf("[回收站] 标记子文件失败: %v", err)
		}
	}

	log.Printf("[回收站] 已移入回收站: %s", fullPath)
	return nil
}

// purgeGhostRecord 清除磁盘上已不存在的残留数据库记录（目录时递归清除子记录）
func purgeGhostRecord(rec *FileRecord) error {
	if err := DeleteFileRecord(rec.FullPath); err != nil {
		return err
	}
	if rec.IsDir {
		prefix := rec.FullPath + string(filepath.Separator)
		if _, err := DB.Exec(`DELETE FROM files WHERE full_path LIKE ?`, prefix+"%"); err != nil {
			return err
		}
	}
	return nil
}

// TrashRestore 从回收站恢复文件
func TrashRestore(uuidStr string) error {
	rec := &FileRecord{}
	var isDir, hasThumb int
	err := DB.QueryRow(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE uuid=? AND status='trashed'
	`, uuidStr).Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
	if err != nil {
		return fmt.Errorf("回收站中未找到该文件: %s", uuidStr)
	}
	rec.IsDir = isDir == 1
	rec.HasThumbnail = hasThumb == 1

	trashName := rec.Name
	if rec.Ext != "" {
		trashName = fmt.Sprintf("%s_%s.%s", strings.TrimSuffix(rec.Name, "."+rec.Ext), rec.UUID, rec.Ext)
	} else {
		trashName = fmt.Sprintf("%s_%s", rec.Name, rec.UUID)
	}
	trashPath := filepath.Join(AppConfig.MountPath, ".trash", trashName)

	os.MkdirAll(rec.ParentPath, 0755)

	if err := os.Rename(trashPath, rec.FullPath); err != nil {
		return fmt.Errorf("恢复文件失败: %w", err)
	}

	if err := MarkFileActive(rec.FullPath); err != nil {
		return fmt.Errorf("更新数据库状态失败: %w", err)
	}

	// 如果是文件夹，恢复所有子文件
	if rec.IsDir {
		prefix := rec.FullPath + string(filepath.Separator)
		_, err := DB.Exec(`UPDATE files SET status='active', deleted_at=NULL WHERE full_path LIKE ? AND status='trashed'`, prefix+"%")
		if err != nil {
			log.Printf("[回收站] 恢复子文件失败: %v", err)
		}
	}

	log.Printf("[回收站] 已恢复: %s", rec.FullPath)
	return nil
}

// TrashDelete 彻底删除文件（从回收站和数据库中删除）
func TrashDelete(uuidStr string) error {
	rec := &FileRecord{}
	var isDir, hasThumb int
	err := DB.QueryRow(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE uuid=? AND status='trashed'
	`, uuidStr).Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
	if err != nil {
		return fmt.Errorf("回收站中未找到该文件: %s", uuidStr)
	}

	trashName := rec.Name
	if rec.Ext != "" {
		trashName = fmt.Sprintf("%s_%s.%s", strings.TrimSuffix(rec.Name, "."+rec.Ext), rec.UUID, rec.Ext)
	} else {
		trashName = fmt.Sprintf("%s_%s", rec.Name, rec.UUID)
	}
	trashPath := filepath.Join(AppConfig.MountPath, ".trash", trashName)

	// 如果是文件夹，递归删除磁盘内容
	if isDir == 1 {
		os.RemoveAll(trashPath)
	} else {
		os.Remove(trashPath)
	}

	// 删除自身数据库记录
	if err := DeleteFileRecord(rec.FullPath); err != nil {
		return fmt.Errorf("删除数据库记录失败: %w", err)
	}

	// 如果是文件夹，删除所有子文件的数据库记录
	if isDir == 1 {
		prefix := rec.FullPath + string(filepath.Separator)
		_, err := DB.Exec(`DELETE FROM files WHERE full_path LIKE ?`, prefix+"%")
		if err != nil {
			log.Printf("[回收站] 删除子文件记录失败: %v", err)
		}
	}

	log.Printf("[回收站] 已彻底删除: %s", rec.Name)
	return nil
}

// TrashList 列出回收站中的文件，支持分页
func TrashList(page, pageSize int) ([]FileRecord, int64, error) {
	var total int64
	DB.QueryRow(`SELECT COUNT(1) FROM files WHERE status='trashed'`).Scan(&total)
	if total == 0 {
		return []FileRecord{}, 0, nil
	}

	offset := 0
	if page > 1 {
		offset = (page - 1) * pageSize
	}

	rows, err := DB.Query(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE status='trashed'
		ORDER BY deleted_at DESC
		LIMIT ? OFFSET ?
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var rec FileRecord
		var isDir, hasThumb int
		err := rows.Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
		if err != nil {
			return nil, 0, err
		}
		rec.IsDir = isDir == 1
		rec.HasThumbnail = hasThumb == 1
		files = append(files, rec)
	}
	return files, total, nil
}

// TrashAutoClean 自动清理回收站中超过指定天数的文件
func TrashAutoClean(days int) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")

	rows, err := DB.Query(`
		SELECT uuid, name, ext, full_path FROM files WHERE status='trashed' AND deleted_at < ?
	`, cutoff)
	if err != nil {
		log.Printf("[回收站] 自动清理查询失败: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var fileUUID, name, ext, fullPath string
		if err := rows.Scan(&fileUUID, &name, &ext, &fullPath); err != nil {
			continue
		}

		// 删除磁盘文件
		trashName := name
		if ext != "" {
			trashName = fmt.Sprintf("%s_%s.%s", strings.TrimSuffix(name, "."+ext), fileUUID, ext)
		} else {
			trashName = fmt.Sprintf("%s_%s", name, fileUUID)
		}
		trashPath := filepath.Join(AppConfig.MountPath, ".trash", trashName)
		os.RemoveAll(trashPath)

		// 删除数据库记录
		DeleteFileRecord(fullPath)
		count++
	}

	if count > 0 {
		log.Printf("[回收站] 自动清理了 %d 个过期文件", count)
	}
}

// TrashEmpty 清空回收站，删除所有已标记为 trashed 的文件
func TrashEmpty() (int64, error) {
	rows, err := DB.Query(`
		SELECT uuid, name, ext, full_path, is_dir FROM files WHERE status='trashed'
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var fileUUID, name, ext, fullPath string
		var isDir int
		if err := rows.Scan(&fileUUID, &name, &ext, &fullPath, &isDir); err != nil {
			continue
		}

		// 删除磁盘文件
		trashName := name
		if ext != "" {
			trashName = fmt.Sprintf("%s_%s.%s", strings.TrimSuffix(name, "."+ext), fileUUID, ext)
		} else {
			trashName = fmt.Sprintf("%s_%s", name, fileUUID)
		}
		trashPath := filepath.Join(AppConfig.MountPath, ".trash", trashName)

		if isDir == 1 {
			os.RemoveAll(trashPath)
			// 删除子文件记录
			prefix := fullPath + string(filepath.Separator)
			DB.Exec(`DELETE FROM files WHERE full_path LIKE ?`, prefix+"%")
		} else {
			os.Remove(trashPath)
		}

		DeleteFileRecord(fullPath)
		count++
	}

	return count, nil
}
