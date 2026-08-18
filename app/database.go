package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// FileRecord 文件记录结构体
type FileRecord struct {
	ID            int64   `json:"id"`
	UUID          string  `json:"uuid"`
	Name          string  `json:"name"`
	ParentPath    string  `json:"parent_path"`
	FullPath      string  `json:"full_path"`
	IsDir         bool    `json:"is_dir"`
	Size          int64   `json:"size"`
	Ext           string  `json:"ext"`
	MD5           string  `json:"md5"`
	Status        string  `json:"status"`
	HasThumbnail  bool    `json:"has_thumbnail"`
	CreatedAt     string  `json:"created_at"`
	DeletedAt     *string `json:"deleted_at,omitempty"`
}

// InitDB 初始化 SQLite 数据库
func InitDB() error {
	dbPath := filepath.Join(DataDir, "pan.db")

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// 连接池配置（SQLite 单文件，限制并发写入）
	DB.SetMaxOpenConns(4)
	DB.SetMaxIdleConns(2)
	DB.SetConnMaxLifetime(0)

	// SQLite 优化配置
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA cache_size=-64000;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := DB.Exec(p); err != nil {
			return fmt.Errorf("设置PRAGMA失败: %w", err)
		}
	}

	// 创建 files 表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		parent_path TEXT NOT NULL,
		full_path TEXT NOT NULL UNIQUE,
		is_dir INTEGER NOT NULL DEFAULT 0,
		size INTEGER NOT NULL DEFAULT 0,
		ext TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		has_thumbnail INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
		deleted_at TEXT,
		updated_at TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_files_parent_path ON files(parent_path);
	CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
	CREATE INDEX IF NOT EXISTS idx_files_uuid ON files(uuid);
	CREATE INDEX IF NOT EXISTS idx_files_full_path ON files(full_path);
	`
	if _, err := DB.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}

	// 兼容旧库：添加 md5 列
	migrations := []string{
		"ALTER TABLE files ADD COLUMN md5 TEXT DEFAULT ''",
		"CREATE INDEX IF NOT EXISTS idx_files_md5 ON files(md5)",
		"CREATE INDEX IF NOT EXISTS idx_files_dir_size ON files(full_path, is_dir, status, size)",
		// 覆盖索引：加速 ListFiles 的 WHERE + ORDER BY + 分页查询
		"CREATE INDEX IF NOT EXISTS idx_files_list ON files(parent_path, status, is_dir DESC, name ASC)",
	}
	for _, m := range migrations {
		DB.Exec(m)
	}

	log.Printf("[数据库] 初始化完成: %s", dbPath)
	return nil
}

// CloseDB 关闭数据库
func CloseDB() {
	if DB != nil {
		DB.Close()
		log.Println("[数据库] 已关闭")
	}
}

// UpsertFile 插入或更新文件记录
func UpsertFile(rec *FileRecord) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`
		INSERT INTO files (uuid, name, parent_path, full_path, is_dir, size, ext, md5, status, has_thumbnail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', 0, ?)
		ON CONFLICT(full_path) DO UPDATE SET
			name=excluded.name,
			size=excluded.size,
			is_dir=excluded.is_dir,
			ext=excluded.ext,
			md5=excluded.md5,
			status='active'
	`, rec.UUID, rec.Name, rec.ParentPath, rec.FullPath, boolToInt(rec.IsDir), rec.Size, rec.Ext, rec.MD5, now)
	return err
}

// BatchUpsertFiles 批量插入或更新文件记录（使用事务提升性能）
func BatchUpsertFiles(records []*FileRecord) error {
	if len(records) == 0 {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO files (uuid, name, parent_path, full_path, is_dir, size, ext, md5, status, has_thumbnail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', 0, ?)
		ON CONFLICT(full_path) DO UPDATE SET
			name=excluded.name,
			size=excluded.size,
			is_dir=excluded.is_dir,
			ext=excluded.ext,
			md5=excluded.md5,
			status='active'
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		_, err := stmt.Exec(rec.UUID, rec.Name, rec.ParentPath, rec.FullPath, boolToInt(rec.IsDir), rec.Size, rec.Ext, rec.MD5, now)
		if err != nil {
			log.Printf("[数据库] 批量写入跳过: %s, %v", rec.FullPath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// MarkFileTrashed 将文件标记为已删除（移入回收站）
func MarkFileTrashed(fullPath string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE files SET status='trashed', deleted_at=? WHERE full_path=?`, now, fullPath)
	return err
}

// MarkFileActive 恢复文件状态为活跃
func MarkFileActive(fullPath string) error {
	_, err := DB.Exec(`UPDATE files SET status='active', deleted_at=NULL WHERE full_path=?`, fullPath)
	return err
}

// DeleteFileRecord 从数据库中彻底删除记录
func DeleteFileRecord(fullPath string) error {
	_, err := DB.Exec(`DELETE FROM files WHERE full_path=?`, fullPath)
	return err
}

// GetFileByUUID 通过UUID查询文件
func GetFileByUUID(uuid string) (*FileRecord, error) {
	rec := &FileRecord{}
	var isDir int
	var hasThumb int
	err := DB.QueryRow(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE uuid=? AND status='active'
	`, uuid).Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
	if err != nil {
		return nil, err
	}
	rec.IsDir = isDir == 1
	rec.HasThumbnail = hasThumb >= 1
	return rec, nil
}

// ListFiles 列出指定目录下的活跃文件，支持分页
func ListFiles(parentPath string, page, pageSize int) ([]FileRecord, int64, error) {
	var total int64
	err := DB.QueryRow(`SELECT COUNT(1) FROM files WHERE parent_path=? AND status='active'`, parentPath).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []FileRecord{}, 0, nil
	}

	offset := 0
	if page > 1 {
		offset = (page - 1) * pageSize
	}

	rows, err := DB.Query(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE parent_path=? AND status='active'
		ORDER BY is_dir DESC, name ASC
		LIMIT ? OFFSET ?
	`, parentPath, pageSize, offset)
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
		rec.HasThumbnail = hasThumb >= 1
		files = append(files, rec)
	}
	return files, total, nil
}

// SetThumbnail 更新缩略图状态 (0=未生成, 1=已生成, 2=生成失败)
func SetThumbnail(fullPath string, status int) error {
	_, err := DB.Exec(`UPDATE files SET has_thumbnail=? WHERE full_path=?`, status, fullPath)
	return err
}

// GetFileByPath 通过完整路径查询文件
func GetFileByPath(fullPath string) (*FileRecord, error) {
	rec := &FileRecord{}
	var isDir int
	var hasThumb int
	err := DB.QueryRow(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE full_path=? AND status='active'
	`, fullPath).Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
	if err != nil {
		return nil, err
	}
	rec.IsDir = isDir == 1
	rec.HasThumbnail = hasThumb >= 1
	return rec, nil
}

// DeleteStaleRecords 删除数据库中存在但磁盘上不存在的记录（事务批量删除）
func DeleteStaleRecords(existingPaths map[string]bool) error {
	rows, err := DB.Query(`SELECT full_path FROM files WHERE status='active'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			continue
		}
		if !existingPaths[fp] {
			toDelete = append(toDelete, fp)
		}
	}

	if len(toDelete) == 0 {
		return nil
	}

	// 事务批量删除，每批500条
	const batchSize = 500
	for i := 0; i < len(toDelete); i += batchSize {
		end := i + batchSize
		if end > len(toDelete) {
			end = len(toDelete)
		}
		batch := toDelete[i:end]

		tx, err := DB.Begin()
		if err != nil {
			log.Printf("[同步] 开始事务失败: %v", err)
			continue
		}

		stmt, err := tx.Prepare(`DELETE FROM files WHERE full_path=?`)
		if err != nil {
			tx.Rollback()
			log.Printf("[同步] 准备语句失败: %v", err)
			continue
		}

		for _, fp := range batch {
			if _, err := stmt.Exec(fp); err != nil {
				log.Printf("[同步] 删除过期记录失败: %s, %v", fp, err)
			}
		}
		stmt.Close()

		if err := tx.Commit(); err != nil {
			log.Printf("[同步] 提交事务失败: %v", err)
		}
	}

	log.Printf("[同步] 清理了 %d 条过期记录", len(toDelete))
	return nil
}

// fileExistsInDB 检查文件路径是否已存在于数据库中
func fileExistsInDB(fullPath string) bool {
	var count int
	DB.QueryRow(`SELECT COUNT(1) FROM files WHERE full_path=? AND status='active'`, fullPath).Scan(&count)
	return count > 0
}

// boolToInt 布尔转整数
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isHiddenFile 判断是否为隐藏文件/目录（跨平台）
func isHiddenFile(name string) bool {
	// Unix 风格隐藏文件（.开头）
	if len(name) > 0 && name[0] == '.' {
		return true
	}
	// Windows 系统特殊目录
	if runtime.GOOS == "windows" {
		upper := strings.ToUpper(name)
		windowsSpecial := []string{"SYSTEM VOLUME INFORMATION", "$RECYCLE.BIN", "RECYCLER"}
		for _, s := range windowsSpecial {
			if upper == s {
				return true
			}
		}
	}
	return false
}

// isTrashPath 判断相对路径是否属于回收站目录（跨平台）
func isTrashPath(relPath string) bool {
	// 统一使用正斜杠比较，兼容 Windows 反斜杠
	normalized := filepath.ToSlash(relPath)
	return normalized == ".trash" || strings.HasPrefix(normalized, ".trash/")
}

// getExt 获取文件扩展名（小写，不含点）
func getExt(name string) string {
	ext := filepath.Ext(name)
	if len(ext) > 0 {
		return strings.ToLower(ext[1:])
	}
	return ""
}

// ensureTrashDir 确保回收站目录存在
func ensureTrashDir() error {
	trashDir := filepath.Join(AppConfig.MountPath, ".trash")
	return os.MkdirAll(trashDir, 0755)
}

// SearchFiles 按关键词模糊搜索活跃文件，支持分页和文件类型过滤
// pathPrefix 为可选的限制范围前缀，为空则搜索全部
// fileType 为可选的文件类型过滤：image, video, audio, document, other
func SearchFiles(keyword, pathPrefix, fileType string, page, pageSize int) ([]FileRecord, int64, error) {
	whereClauses := []string{"status='active'", "name LIKE ?"}
	args := []interface{}{"%" + keyword + "%"}

	if pathPrefix != "" {
		absPrefix := filepath.Join(AppConfig.MountPath, filepath.FromSlash(pathPrefix)) + string(os.PathSeparator)
		whereClauses = append(whereClauses, "full_path LIKE ?")
		args = append(args, absPrefix+"%")
	}

	extCondition := buildFileTypeCondition(fileType)
	if extCondition != "" {
		whereClauses = append(whereClauses, extCondition)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	var total int64
	countSQL := "SELECT COUNT(1) FROM files WHERE " + whereSQL
	DB.QueryRow(countSQL, args...).Scan(&total)

	offset := 0
	if page > 1 {
		offset = (page - 1) * pageSize
	}

	querySQL := fmt.Sprintf(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE %s
		ORDER BY is_dir DESC, name ASC
		LIMIT ? OFFSET ?
	`, whereSQL)
	queryArgs := append(args, pageSize, offset)

	rows, err := DB.Query(querySQL, queryArgs...)
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
		rec.HasThumbnail = hasThumb >= 1
		files = append(files, rec)
	}
	return files, total, nil
}

// buildFileTypeCondition 根据文件类型构建 SQL WHERE 条件
func buildFileTypeCondition(fileType string) string {
	switch fileType {
	case "image":
		imageExts := []string{"jpg", "jpeg", "png", "gif", "bmp", "webp", "tiff", "tif"}
		return buildExtInClause(imageExts)
	case "video":
		videoExts := []string{"mp4", "avi", "mkv", "mov", "wmv", "flv", "webm", "m4v", "mpeg", "mpg", "3gp"}
		return buildExtInClause(videoExts)
	case "audio":
		audioExts := []string{"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a"}
		return buildExtInClause(audioExts)
	case "document":
		docExts := []string{"doc", "docx", "pdf", "txt", "xls", "xlsx", "ppt", "pptx", "md", "csv", "rtf", "odt", "ods", "odp"}
		return buildExtInClause(docExts)
	case "other":
		allExts := []string{"jpg", "jpeg", "png", "gif", "bmp", "webp", "tiff", "tif",
			"mp4", "avi", "mkv", "mov", "wmv", "flv", "webm", "m4v", "mpeg", "mpg", "3gp",
			"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a",
			"doc", "docx", "pdf", "txt", "xls", "xlsx", "ppt", "pptx", "md", "csv", "rtf", "odt", "ods", "odp"}
		return "NOT " + buildExtInClause(allExts)
	default:
		return ""
	}
}

// buildExtInClause 构建 ext IN (...) SQL 子句
func buildExtInClause(exts []string) string {
	quoted := make([]string, len(exts))
	for i, e := range exts {
		quoted[i] = "'" + e + "'"
	}
	return "ext IN (" + strings.Join(quoted, ",") + ")"
}

// ListFilesSorted 列出指定目录下的活跃文件，支持排序、类型过滤和分页
// sortBy 白名单：name, size, created_at；sortOrder 白名单：asc, desc
func ListFilesSorted(parentPath, sortBy, sortOrder, fileType string, page, pageSize int) ([]FileRecord, int64, error) {
	allowedSortBy := map[string]string{
		"name":       "name",
		"size":       "size",
		"created_at": "created_at",
	}
	allowedOrder := map[string]string{
		"asc":  "ASC",
		"desc": "DESC",
	}

	col, ok := allowedSortBy[sortBy]
	if !ok {
		col = "name"
	}
	dir, ok := allowedOrder[sortOrder]
	if !ok {
		dir = "ASC"
	}

	extCondition := buildFileTypeCondition(fileType)
	whereExtra := ""
	if extCondition != "" {
		whereExtra = " AND " + extCondition
	}

	var total int64
	countSQL := "SELECT COUNT(1) FROM files WHERE parent_path=? AND status='active'" + whereExtra
	DB.QueryRow(countSQL, parentPath).Scan(&total)
	if total == 0 {
		return []FileRecord{}, 0, nil
	}

	offset := 0
	if page > 1 {
		offset = (page - 1) * pageSize
	}

	query := fmt.Sprintf(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE parent_path=? AND status='active'%s
		ORDER BY is_dir DESC, %s %s
		LIMIT ? OFFSET ?
	`, whereExtra, col, dir)

	rows, err := DB.Query(query, parentPath, pageSize, offset)
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
		rec.HasThumbnail = hasThumb >= 1
		files = append(files, rec)
	}
	return files, total, nil
}

// UpdateFileRename 更新文件重命名后的数据库记录
// oldFullPath 原始完整路径，newFullPath 新的完整路径，newName 新文件名，newExt 新扩展名
func UpdateFileRename(oldFullPath, newFullPath, newName, newExt string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE files SET name=?, full_path=?, ext=?, updated_at=? WHERE full_path=?`, newName, newFullPath, newExt, now, oldFullPath)
	return err
}

// UpdateFileMove 更新文件移动后的数据库记录（full_path 和 parent_path）
func UpdateFileMove(oldFullPath, newFullPath, newParentPath string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE files SET full_path=?, parent_path=?, updated_at=? WHERE full_path=?`, newFullPath, newParentPath, now, oldFullPath)
	return err
}

// GetDirSize 递归计算目录内所有活跃文件的总大小
func GetDirSize(dirPath string) (int64, error) {
	var totalSize int64
	prefix := dirPath + string(os.PathSeparator)
	prefixEnd := dirPath + string(os.PathSeparator+1)
	err := DB.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM files WHERE full_path >= ? AND full_path < ? AND is_dir=0 AND status='active'`, prefix, prefixEnd).Scan(&totalSize)
	return totalSize, err
}

// 目录大小缓存（path → {size, expireAt}）
var dirSizeCache sync.Map

type dirSizeCacheEntry struct {
	Size     int64
	ExpireAt time.Time
}

const dirSizeCacheTTL = 5 * time.Minute

// InvalidateDirSizeCache 清除指定目录及其所有上级目录的大小缓存
func InvalidateDirSizeCache(dirPath string) {
	dirSizeCache.Delete(dirPath)
	for p := filepath.Dir(dirPath); p != dirPath && p != "." && p != "/"; p = filepath.Dir(p) {
		dirSizeCache.Delete(p)
		dirPath = p
	}
}

// GetDirSizesBatch 批量查询多个目录的大小，带内存缓存避免重复计算
func GetDirSizesBatch(dirPaths []string) map[string]int64 {
	result := make(map[string]int64, len(dirPaths))
	if len(dirPaths) == 0 {
		return result
	}

	now := time.Now()
	var needQuery []string
	for _, dp := range dirPaths {
		if v, ok := dirSizeCache.Load(dp); ok {
			entry := v.(dirSizeCacheEntry)
			if now.Before(entry.ExpireAt) {
				result[dp] = entry.Size
				continue
			}
		}
		needQuery = append(needQuery, dp)
	}

	if len(needQuery) == 0 {
		return result
	}

	// 使用 UNION ALL 查询，每批最多 50 个
	// 用范围查询替代 LIKE，利用 idx_files_dir_size 索引
	batchSize := 50
	for i := 0; i < len(needQuery); i += batchSize {
		end := i + batchSize
		if end > len(needQuery) {
			end = len(needQuery)
		}
		batch := needQuery[i:end]

		var sb strings.Builder
		args := make([]interface{}, 0, len(batch)*3)
		for j, dp := range batch {
			if j > 0 {
				sb.WriteString(" UNION ALL ")
			}
			// full_path >= 'dir/' AND full_path < 'dir0' 等价于 LIKE 'dir/%'，但能更好利用B-tree索引
			prefix := dp + string(os.PathSeparator)
			prefixEnd := dp + string(os.PathSeparator+1) // 路径分隔符的下一个字符作为范围上界
			sb.WriteString("SELECT ? AS dir_path, COALESCE(SUM(size), 0) AS total_size FROM files WHERE full_path >= ? AND full_path < ? AND is_dir=0 AND status='active'")
			args = append(args, dp, prefix, prefixEnd)
		}

		rows, err := DB.Query(sb.String(), args...)
		if err != nil {
			log.Printf("[数据库] 批量查询目录大小失败: %v", err)
			continue
		}

		expireAt := now.Add(dirSizeCacheTTL)
		for rows.Next() {
			var dirPath string
			var totalSize int64
			if rows.Scan(&dirPath, &totalSize) == nil {
				result[dirPath] = totalSize
				dirSizeCache.Store(dirPath, dirSizeCacheEntry{Size: totalSize, ExpireAt: expireAt})
			}
		}
		rows.Close()
	}
	return result
}

// UpdateChildrenPaths 更新所有子文件的完整路径（文件夹重命名时使用）
func UpdateChildrenPaths(oldPrefix, newPrefix string) error {
	_, err := DB.Exec(`UPDATE files SET full_path = REPLACE(full_path, ?, ?), parent_path = REPLACE(parent_path, ?, ?) WHERE full_path LIKE ? AND status='active'`, oldPrefix, newPrefix, oldPrefix, newPrefix, oldPrefix+string(os.PathSeparator)+"%")
	return err
}

// ComputeFileMD5 计算文件 MD5
func ComputeFileMD5(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FindDuplicate 查找同目录下同 MD5+大小的文件
// 返回：exactMatch(同名完全一致), contentMatch(不同名但内容一致)
func FindDuplicate(parentPath, name, md5 string, size int64) (exactMatch *FileRecord, contentMatch *FileRecord) {
	if md5 == "" {
		return nil, nil
	}

	rows, err := DB.Query(`
		SELECT id, uuid, name, parent_path, full_path, is_dir, size, ext, md5, status, has_thumbnail, created_at, deleted_at
		FROM files WHERE parent_path=? AND md5=? AND size=? AND is_dir=0 AND status='active'
	`, parentPath, md5, size)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		var rec FileRecord
		var isDir, hasThumb int
		rows.Scan(&rec.ID, &rec.UUID, &rec.Name, &rec.ParentPath, &rec.FullPath, &isDir, &rec.Size, &rec.Ext, &rec.MD5, &rec.Status, &hasThumb, &rec.CreatedAt, &rec.DeletedAt)
		rec.IsDir = isDir == 1
		rec.HasThumbnail = hasThumb >= 1
		if rec.Name == name {
			exactMatch = &rec
		} else if contentMatch == nil {
			contentMatch = &rec
		}
	}
	return exactMatch, contentMatch
}

// UpdateFileMD5 更新文件的 MD5 值
func UpdateFileMD5(fullPath, md5 string) {
	DB.Exec(`UPDATE files SET md5=? WHERE full_path=?`, md5, fullPath)
}
