package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// APIResponse 统一响应结构
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// FileListResp 文件列表响应
type FileListResp struct {
	Path     string      `json:"path"`
	Files    interface{} `json:"files"`
	Parent   string      `json:"parent"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// FileItem 文件列表项（用于 API 响应）
type FileItem struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	RelPath      string `json:"rel_path"`
	IsDir        bool   `json:"is_dir"`
	Size         int64  `json:"size"`
	Ext          string `json:"ext"`
	Status       string `json:"status"`
	HasThumbnail bool   `json:"has_thumbnail"`
	CreatedAt    string `json:"created_at"`
}

// Router API 路由器
var Router *mux.Router

// repairLocks 防止同一文件被并发修复
var repairLocks sync.Map

// toRelPath 将绝对路径转为相对挂载目录的相对路径
func toRelPath(absPath string) string {
	rel, err := filepath.Rel(AppConfig.MountPath, absPath)
	if err != nil {
		return ""
	}
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// toAbsPath 将相对路径转为绝对路径（挂载目录下）
func toAbsPath(relPath string) string {
	if relPath == "" || relPath == "/" {
		return AppConfig.MountPath
	}
	return filepath.Join(AppConfig.MountPath, filepath.FromSlash(relPath))
}

// InitAPI 初始化 API 路由
func InitAPI() http.Handler {
	Router = mux.NewRouter()

	// 请求日志中间件 + CORS
	Router.Use(loggingMiddleware)
	Router.Use(corsMiddleware)

	// API 路由
	api := Router.PathPrefix("/api").Subrouter()
	api.Use(authMiddleware)
	api.HandleFunc("/list", handleList).Methods("GET", "OPTIONS")
	api.HandleFunc("/thumb", handleThumb).Methods("GET", "OPTIONS")
	api.HandleFunc("/download", handleDownload).Methods("GET", "OPTIONS")
	api.HandleFunc("/download/zip", handleZipDownload).Methods("GET", "OPTIONS")
	api.HandleFunc("/stream", handleStream).Methods("GET", "OPTIONS")
	api.HandleFunc("/probe", handleProbe).Methods("GET", "OPTIONS")
	api.HandleFunc("/transcode", handleTranscode).Methods("GET", "OPTIONS")
	api.HandleFunc("/upload", handleUpload).Methods("POST", "OPTIONS")
	api.HandleFunc("/upload/chunk", handleUploadChunkInit).Methods("POST", "OPTIONS")
	api.HandleFunc("/upload/chunk/{uploadId}", handleUploadChunk).Methods("POST", "OPTIONS")
	api.HandleFunc("/upload/chunk/{uploadId}/complete", handleUploadChunkComplete).Methods("POST", "OPTIONS")
	api.HandleFunc("/upload/chunk/{uploadId}/status", handleUploadChunkStatus).Methods("GET", "OPTIONS")
	api.HandleFunc("/mkdir", handleMkdir).Methods("POST", "OPTIONS")
	api.HandleFunc("/trash", handleTrashList).Methods("GET", "OPTIONS")
	api.HandleFunc("/trash/restore", handleTrashRestore).Methods("POST", "OPTIONS")
	api.HandleFunc("/trash/delete", handleTrashDelete).Methods("POST", "OPTIONS")
	api.HandleFunc("/trash/empty", handleTrashEmpty).Methods("POST", "OPTIONS")
	api.HandleFunc("/delete", handleDelete).Methods("POST", "OPTIONS")
	api.HandleFunc("/batch/delete", handleBatchDelete).Methods("POST", "OPTIONS")
	api.HandleFunc("/search", handleSearch).Methods("GET", "OPTIONS")
	api.HandleFunc("/rename", handleRename).Methods("POST", "OPTIONS")
	api.HandleFunc("/move", handleMove).Methods("POST", "OPTIONS")
	api.HandleFunc("/copy", handleCopy).Methods("POST", "OPTIONS")
	api.HandleFunc("/stats", handleStats).Methods("GET", "OPTIONS")
	api.HandleFunc("/file/info", handleFileInfo).Methods("GET", "OPTIONS")
	api.HandleFunc("/file/regen-thumb", handleRegenThumbnail).Methods("POST", "OPTIONS")
	api.HandleFunc("/video/repair", handleVideoRepair).Methods("POST", "OPTIONS")
	api.HandleFunc("/local-play-token", handleLocalPlayToken).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/dashboard", handleAdminDashboard).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/thumb/failed", handleAdminThumbFailed).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/thumb/retry-all", handleAdminThumbRetryAll).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/thumb/retry", handleAdminThumbRetry).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/thumb/generate-pending", handleAdminThumbGeneratePending).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/config", handleAdminGetConfig).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/config", handleAdminSetConfig).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/logs", handleAdminLogs).Methods("GET", "OPTIONS")
	api.HandleFunc("/auth/login", handleLogin).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/logout", handleLogout).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/check", handleAuthCheck).Methods("GET", "OPTIONS")

	RegisterFrontend(Router)

	log.Println("[API] 路由初始化完成")
	return Router
}

// loggingMiddleware 请求日志中间件
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		remoteAddr := r.RemoteAddr

		// 包装 ResponseWriter 以捕获状态码
		wrapped := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// 跳过前端静态资源的日志（减少噪音）
		path := r.URL.Path
		if len(path) >= 8 && path[:8] == "/assets/" {
			return
		}

		duration := time.Since(start)
		log.Printf("[HTTP] %s %s %s -> %d (%v)",
			remoteAddr, r.Method, path, wrapped.statusCode, duration)
	})
}

// cleanStaleChunkUploads 清理超时的分片上传任务
func cleanStaleChunkUploads(maxAge time.Duration) {
	entries, err := os.ReadDir(DataDir)
	if err != nil {
		return
	}

	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".meta") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > maxAge {
			uploadId := strings.TrimSuffix(name, ".meta")
			os.Remove(filepath.Join(DataDir, name))
			os.RemoveAll(filepath.Join(DataDir, uploadId))
			count++
		}
	}
	if count > 0 {
		log.Printf("[上传] 清理了 %d 个超时分片上传任务", count)
	}
}

// responseWriterWrapper 包装 ResponseWriter 以捕获状态码
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// corsMiddleware 跨域中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Content-Range")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// respondJSON 返回 JSON 响应
func respondJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(APIResponse{
		Code:    code,
		Message: "success",
		Data:    data,
	})
}

// respondError 返回错误响应
func respondError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(APIResponse{
		Code:    code,
		Message: msg,
	})
}

// handleList 文件列表接口
// GET /api/list?path=photos/2024&sort=name&order=asc&page=1&page_size=100
func handleList(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	absPath := toAbsPath(relPath)

	cleanPath, err := sanitizePath(AppConfig.MountPath, relPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "非法路径")
		return
	}
	_ = cleanPath

	// 优先从数据库检查目录是否存在（索引查询极快），避免树莓派上的磁盘IO
	if relPath != "" {
		var exists int
		DB.QueryRow(`SELECT 1 FROM files WHERE full_path=? AND is_dir=1 AND status='active' LIMIT 1`, absPath).Scan(&exists)
		if exists != 1 {
			// 数据库没有记录时 fallback 到文件系统检查
			info, err := os.Stat(absPath)
			if err != nil || !info.IsDir() {
				respondError(w, http.StatusNotFound, "目录不存在")
				return
			}
		}
	}

	sortBy := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order")
	fileType := r.URL.Query().Get("type")

	page := 1
	pageSize := 100
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 500 {
			pageSize = v
		}
	}

	var files []FileRecord
	var total int64
	if sortBy != "" || sortOrder != "" || fileType != "" {
		files, total, err = ListFilesSorted(absPath, sortBy, sortOrder, fileType, page, pageSize)
	} else {
		files, total, err = ListFiles(absPath, page, pageSize)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "查询失败")
		return
	}

	if files == nil {
		files = []FileRecord{}
	}

	var parentRel string
	if relPath != "" && relPath != "/" {
		parentRel = filepath.ToSlash(filepath.Dir(relPath))
		if parentRel == "." {
			parentRel = ""
		}
	}

	items := make([]FileItem, 0, len(files))
	for _, f := range files {
		item := FileItem{
			UUID:         f.UUID,
			Name:         f.Name,
			RelPath:      toRelPath(f.FullPath),
			IsDir:        f.IsDir,
			Size:         f.Size,
			Ext:          f.Ext,
			Status:       f.Status,
			HasThumbnail: f.HasThumbnail,
			CreatedAt:    f.CreatedAt,
		}
		items = append(items, item)
	}

	respondJSON(w, http.StatusOK, FileListResp{
		Path:      relPath,
		Files:     items,
		Parent:    parentRel,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	})
}

// handleThumb 缩略图接口
// GET /api/thumb?uuid=xxx
func handleThumb(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	thumbPath := GetThumbnailPath(uuidStr)
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		http.Error(w, "缩略图不存在", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	http.ServeFile(w, r, thumbPath)
}

// contentDispositionName 生成兼容中文的 Content-Disposition 文件名
func contentDispositionName(name string) string {
	escaped := url.PathEscape(name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, escaped, escaped)
}

// handleDownload 文件下载接口
// GET /api/download?uuid=xxx
func handleDownload(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	log.Printf("[下载] %s (%s) 来自 %s", rec.Name, formatFileSize(rec.Size), r.RemoteAddr)

	w.Header().Set("Content-Disposition", contentDispositionName(rec.Name))
	w.Header().Set("Content-Type", detectContentType(rec.FullPath))
	http.ServeFile(w, r, rec.FullPath)
}

// handleStream 流式播放接口（支持 Range Requests）
// GET /api/stream?uuid=xxx
func handleStream(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		log.Printf("[流播放] %s Range: %s", rec.Name, rangeHeader)
	} else {
		log.Printf("[流播放] %s (%s) 完整请求", rec.Name, formatFileSize(rec.Size))
	}

	serveFileRange(w, r, rec.FullPath)
}

// handleLocalPlayToken 返回认证令牌，供前端拼装外部播放器（PotPlayer 等）直链
// 需通过认证才能访问；未启用认证时返回空字符串
// GET /api/local-play-token
func handleLocalPlayToken(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"token": AppConfig.AuthToken})
}

// tailBuffer 只保留末尾内容的日志缓冲（用于截取 ffmpeg 错误信息）
type tailBuffer struct {
	buf []byte
}

const tailLimit = 8 * 1024

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > tailLimit {
		t.buf = t.buf[len(t.buf)-tailLimit:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	return string(t.buf)
}

// ffprobeExe 查找 ffprobe：优先取 ffmpeg 同目录，其次 PATH；找不到返回空
func ffprobeExe() string {
	if AppConfig.FFmpegPath == "" {
		return ""
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	// 若 ffmpeg_path 是命令名（如 "ffmpeg"），Dir 为 "."，Join 后仍是命令名，LookPath 会搜索 PATH
	cand := filepath.Join(filepath.Dir(AppConfig.FFmpegPath), "ffprobe"+suffix)
	if p, err := exec.LookPath(cand); err == nil {
		return p
	}
	if p, err := exec.LookPath("ffprobe" + suffix); err == nil {
		return p
	}
	return ""
}

// handleProbe 探测媒体编码信息，前端据此选择直连/转封装/转码播放策略
// GET /api/probe?uuid=xxx
func handleProbe(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	probeExe := ffprobeExe()
	if probeExe == "" {
		respondJSON(w, http.StatusOK, map[string]interface{}{"available": false})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, probeExe,
		"-v", "quiet",
		"-print_format", "json",
		"-show_entries", "stream=index,codec_type,codec_name,codec_tag_string:format=duration",
		rec.FullPath,
	)
	out, err := cmd.Output()
	if err != nil {
		// 探测失败不阻断播放，前端将退回按扩展名判断
		respondJSON(w, http.StatusOK, map[string]interface{}{"available": false})
		return
	}

	var probeRes struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			CodecTag  string `json:"codec_tag_string"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probeRes); err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"available": false})
		return
	}

	videoCodec, videoTag, audioCodec := "", "", ""
	for _, s := range probeRes.Streams {
		if s.CodecType == "video" && videoCodec == "" {
			videoCodec = s.CodecName
			videoTag = s.CodecTag
		} else if s.CodecType == "audio" && audioCodec == "" {
			audioCodec = s.CodecName
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"available":   true,
		"video_codec": videoCodec,
		"video_tag":   videoTag,
		"audio_codec": audioCodec,
		"duration":    probeRes.Format.Duration,
	})
}

// handleTranscode 实时转封装/转码视频为 fMP4 流（用于浏览器不支持的容器/编码格式）
// GET /api/transcode?uuid=xxx&mode=copy|audio|transcode
//   copy      仅转封装：容器不兼容但编码兼容（如 mkv 内的 H.264+AAC），CPU 开销极低
//   audio     视频直拷 + 音频转 AAC：视频编码兼容但音轨不兼容（如 H.264 + AC3/DTS）
//   transcode 完全转码：视频转 H.264 + 音频转 AAC（如 HEVC 片源，CPU 开销最高）
func handleTranscode(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	if AppConfig.FFmpegPath == "" {
		respondError(w, http.StatusServiceUnavailable, "未配置 ffmpeg")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "copy"
	}

	log.Printf("[转码] %s (%s) 模式: %s", rec.Name, rec.Ext, mode)

	// 统一只保留第一路视频流和第一路音频流，丢弃字幕/附件/封面图等，
	// 避免字幕流（ASS/PGS）或附件（mkv 内封封面）导致 MP4 封装失败
	streamArgs := []string{"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn"}

	var codecArgs []string
	switch mode {
	case "audio":
		codecArgs = []string{"-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-ac", "2"}
	case "transcode":
		codecArgs = []string{
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "27", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192k", "-ac", "2",
		}
	default:
		// copy 模式：只转封装，不重新编码
		mode = "copy"
		codecArgs = []string{"-c", "copy"}
	}

	args := append([]string{"-nostdin", "-nostats", "-loglevel", "error", "-i", rec.FullPath}, streamArgs...)
	args = append(args, codecArgs...)
	args = append(args,
		// empty_moov+frag_keyframe+default_base_moof：moov 前置 + 关键帧分片，
		// 浏览器可边下边播。注：旧写法 frag_mp4 在 FFmpeg 8+ 已移除，此处用全版本兼容组合
		"-movflags", "empty_moov+frag_keyframe+default_base_moof",
		"-f", "mp4",
		"-threads", "2",
		"pipe:1",
	)

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Hour)
	defer cancel()

	cmd := exec.CommandContext(ctx, AppConfig.FFmpegPath, args...)
	var errBuf tailBuffer
	cmd.Stderr = &errBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[转码] 创建管道失败: %v", err)
		respondError(w, http.StatusInternalServerError, "转码启动失败")
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[转码] 启动 ffmpeg 失败: %v", err)
		respondError(w, http.StatusInternalServerError, "转码启动失败")
		return
	}

	// 先等第一段输出：若 ffmpeg 立即失败（损坏文件/无法识别的流），
	// 在写 200 响应头之前返回 500，前端能拿到明确错误并逐级降级
	buf := make([]byte, 64*1024) // 64KB 缓冲
	firstN, _ := stdout.Read(buf)
	if firstN == 0 {
		waitErr := cmd.Wait()
		reason := strings.TrimSpace(errBuf.String())
		if reason == "" && waitErr != nil {
			reason = waitErr.Error()
		}
		log.Printf("[转码] ffmpeg 无输出即退出: %s 模式 %s: %s", rec.Name, mode, reason)
		msg := reason
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		respondError(w, http.StatusInternalServerError, "ffmpeg 处理失败: "+msg)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// 流式传输 ffmpeg 输出到 HTTP 响应
	total := int64(0)
	flusher, canFlush := w.(http.Flusher)
	writeChunk := func(p []byte) bool {
		if _, writeErr := w.Write(p); writeErr != nil {
			log.Printf("[转码] 写入中断（客户端断开）: %v", writeErr)
			cmd.Process.Kill()
			return false
		}
		total += int64(len(p))
		if canFlush {
			flusher.Flush()
		}
		return true
	}

	if !writeChunk(buf[:firstN]) {
		cmd.Wait()
		return
	}

	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if !writeChunk(buf[:n]) {
				break
			}
		}
		if readErr != nil {
			break
		}
	}

	cmd.Wait()
	log.Printf("[转码] 完成: %s 模式 %s, 输出 %s", rec.Name, mode, formatFileSize(total))
}

// handleUpload 文件上传接口（流式处理，不经过 ParseMultipartForm 临时文件）
// POST /api/upload?path=photos/2024
func handleUpload(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	absDir := toAbsPath(relPath)

	_, err := sanitizePath(AppConfig.MountPath, relPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "非法路径")
		return
	}

	if err := os.MkdirAll(absDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建目录失败")
		return
	}

	contentType := r.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Printf("[上传] 解析 Content-Type 失败: %v", err)
		respondError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])

	displayPath := relPath
	if displayPath == "" {
		displayPath = "/"
	}

	type UploadResult struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Renamed bool   `json:"renamed,omitempty"`
		Skipped bool   `json:"skipped,omitempty"`
	}

	var results []UploadResult
	fileCount := 0

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[上传] 读取 multipart 失败: %v", err)
			break
		}

		if part.FormName() != "files" {
			part.Close()
			continue
		}

		safeName := filepath.Base(part.FileName())
		if safeName == "." || safeName == "" {
			part.Close()
			continue
		}

		fileCount++

		tmpPath := filepath.Join(absDir, ".upload_"+safeName+"_"+uuid.New().String()[:8])
		MarkUploading(tmpPath)

		hash := md5.New()
		dst, err := os.Create(tmpPath)
		if err != nil {
			part.Close()
			ClearUploading(tmpPath)
			log.Printf("[上传] 创建临时文件失败: %s, %v", safeName, err)
			continue
		}

		written, copyErr := io.Copy(dst, io.TeeReader(part, hash))
		dst.Close()
		part.Close()

		if copyErr != nil {
			os.Remove(tmpPath)
			ClearUploading(tmpPath)
			log.Printf("[上传] 写入文件失败: %s, %v", safeName, copyErr)
			continue
		}

		fileMD5 := hex.EncodeToString(hash.Sum(nil))

		exactDup, contentDup := FindDuplicate(absDir, safeName, fileMD5, written)

		if exactDup != nil || contentDup != nil {
			os.Remove(tmpPath)
			ClearUploading(tmpPath)
			existingName := safeName
			if contentDup != nil {
				existingName = contentDup.Name
			}
			results = append(results, UploadResult{Name: safeName, Size: written, Skipped: true})
			log.Printf("[上传] 跳过重复: %s (已存在: %s)", safeName, existingName)
			continue
		}

		finalName := safeName
		renamed := false
		dstPath := filepath.Join(absDir, safeName)

		if _, err := os.Stat(dstPath); err == nil {
			dstPath = uniqueFilePath(absDir, safeName)
			finalName = filepath.Base(dstPath)
			renamed = true
		}

		ClearUploading(tmpPath)
		if err := os.Rename(tmpPath, dstPath); err != nil {
			log.Printf("[上传] 重命名临时文件失败: %s, %v", safeName, err)
			continue
		}
		MarkUploading(dstPath)

		rec := &FileRecord{
			UUID:       uuid.New().String(),
			Name:       finalName,
			ParentPath: absDir,
			FullPath:   dstPath,
			IsDir:      false,
			Size:       written,
			Ext:        getExt(finalName),
			MD5:        fileMD5,
		}
		UpsertFile(rec)
		SubmitThumbnailTask(dstPath)
		ClearUploading(dstPath)

		results = append(results, UploadResult{
			Name:    finalName,
			Size:    written,
			Renamed: renamed,
		})

		log.Printf("[上传] 完成: %s (%s)", finalName, formatFileSize(written))
	}

	if fileCount == 0 {
		respondError(w, http.StatusBadRequest, "没有选择文件")
		return
	}

	log.Printf("[上传] 上传完成: %d 个文件到 %s (来自 %s)", fileCount, displayPath, r.RemoteAddr)
	InvalidateDirSizeCache(absDir)
	respondJSON(w, http.StatusOK, results)
}

// handleMkdir 创建目录接口
// POST /api/mkdir {"path": "photos/2024", "name": "new_folder"}
func handleMkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.Name == "" {
		respondError(w, http.StatusBadRequest, "目录名不能为空")
		return
	}

	safeName := filepath.Base(body.Name)
	absDir := toAbsPath(body.Path)
	newDir := filepath.Join(absDir, safeName)

	_, err := sanitizePath(AppConfig.MountPath, filepath.Join(body.Path, safeName))
	if err != nil {
		respondError(w, http.StatusBadRequest, "非法路径")
		return
	}

	if err := os.MkdirAll(newDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建目录失败")
		return
	}

	rec := &FileRecord{
		UUID:       uuid.New().String(),
		Name:       safeName,
		ParentPath: absDir,
		FullPath:   newDir,
		IsDir:      true,
		Size:       0,
		Ext:        "",
	}
	UpsertFile(rec)

	log.Printf("[新建目录] %s", toRelPath(newDir))
	respondJSON(w, http.StatusOK, nil)
}

// handleTrashList 回收站列表（支持分页）
func handleTrashList(w http.ResponseWriter, r *http.Request) {
	log.Printf("[回收站] 查看回收站列表 (来自 %s)", r.RemoteAddr)

	page := 1
	pageSize := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 200 {
			pageSize = v
		}
	}

	files, total, err := TrashList(page, pageSize)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "查询回收站失败")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"files":     files,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleTrashRestore 恢复文件
// POST /api/trash/restore {"uuid": "xxx"}
func handleTrashRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	log.Printf("[回收站] 恢复文件: uuid=%s", body.UUID)

	if err := TrashRestore(body.UUID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, nil)
}

// handleTrashDelete 彻底删除
// POST /api/trash/delete {"uuid": "xxx"}
func handleTrashDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	log.Printf("[回收站] 彻底删除: uuid=%s", body.UUID)

	if err := TrashDelete(body.UUID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, nil)
}

// handleTrashEmpty 清空回收站
// POST /api/trash/empty
func handleTrashEmpty(w http.ResponseWriter, r *http.Request) {
	log.Printf("[回收站] 清空回收站 (来自 %s)", r.RemoteAddr)

	count, err := TrashEmpty()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "清空回收站失败: "+err.Error())
		return
	}

	log.Printf("[回收站] 已清空: %d 个文件", count)
	respondJSON(w, http.StatusOK, map[string]int64{"count": count})
}

// handleDelete 删除文件（移入回收站）
// POST /api/delete {"uuid": "xxx"}
func handleDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	log.Printf("[删除] 移入回收站: %s (来自 %s)", rec.Name, r.RemoteAddr)

	if err := TrashMove(rec.FullPath); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	InvalidateDirSizeCache(rec.ParentPath)
	respondJSON(w, http.StatusOK, nil)
}

// handleBatchDelete 批量删除文件（移入回收站）
// POST /api/batch/delete {"uuids": ["xxx", "yyy"]}
func handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUIDs []string `json:"uuids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if len(body.UUIDs) == 0 {
		respondError(w, http.StatusBadRequest, "未选择文件")
		return
	}

	log.Printf("[批量删除] 开始删除 %d 个文件 (来自 %s)", len(body.UUIDs), r.RemoteAddr)

	type BatchResult struct {
		UUID    string `json:"uuid"`
		Name    string `json:"name"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}

	var results []BatchResult
	successCount := 0

	for _, id := range body.UUIDs {
		rec, err := GetFileByUUID(id)
		if err != nil {
			results = append(results, BatchResult{UUID: id, Name: "", Success: false, Error: "文件不存在"})
			continue
		}

		if err := TrashMove(rec.FullPath); err != nil {
			results = append(results, BatchResult{UUID: id, Name: rec.Name, Success: false, Error: err.Error()})
			continue
		}

		results = append(results, BatchResult{UUID: id, Name: rec.Name, Success: true})
		successCount++
	}

	log.Printf("[批量删除] 完成: %d/%d 成功", successCount, len(body.UUIDs))
	respondJSON(w, http.StatusOK, results)
}

// formatFileSize 格式化文件大小
func formatFileSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	fsize := float64(size)
	unit := 0
	for fsize >= 1024 && unit < len(units)-1 {
		fsize /= 1024
		unit++
	}
	return strconv.FormatFloat(fsize, 'f', 2, 64) + " " + units[unit]
}

// handleSearch 文件搜索接口
// GET /api/search?keyword=xxx&path=xxx&type=image&page=1&page_size=100
func handleSearch(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		respondError(w, http.StatusBadRequest, "缺少 keyword 参数")
		return
	}

	pathPrefix := r.URL.Query().Get("path")
	if pathPrefix != "" {
		if _, err := sanitizePath(AppConfig.MountPath, pathPrefix); err != nil {
			respondError(w, http.StatusBadRequest, "非法路径")
			return
		}
	}

	fileType := r.URL.Query().Get("type")
	page := 1
	pageSize := 100
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 500 {
			pageSize = v
		}
	}

	files, total, err := SearchFiles(keyword, pathPrefix, fileType, page, pageSize)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "搜索失败")
		return
	}
	if files == nil {
		files = []FileRecord{}
	}

	items := make([]FileItem, 0, len(files))
	for _, f := range files {
		item := FileItem{
			UUID:         f.UUID,
			Name:         f.Name,
			RelPath:      toRelPath(f.FullPath),
			IsDir:        f.IsDir,
			Size:         f.Size,
			Ext:          f.Ext,
			Status:       f.Status,
			HasThumbnail: f.HasThumbnail,
			CreatedAt:    f.CreatedAt,
		}
		items = append(items, item)
	}

	log.Printf("[搜索] 关键词: %s, 范围: %s, 类型: %s, 结果: %d/%d 条", keyword, pathPrefix, fileType, len(items), total)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"files":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleRename 文件重命名接口
// POST /api/rename {"uuid": "xxx", "new_name": "新名称.jpg"}
func handleRename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID    string `json:"uuid"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.UUID == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid")
		return
	}
	if body.NewName == "" {
		respondError(w, http.StatusBadRequest, "新名称不能为空")
		return
	}

	safeName := filepath.Base(body.NewName)
	if safeName != body.NewName {
		respondError(w, http.StatusBadRequest, "新名称不能包含路径分隔符")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	parentDir := filepath.Dir(rec.FullPath)
	newFullPath := filepath.Join(parentDir, safeName)

	if _, err := sanitizePath(AppConfig.MountPath, filepath.Join(toRelPath(parentDir), safeName)); err != nil {
		respondError(w, http.StatusBadRequest, "新路径非法")
		return
	}

	if newFullPath == rec.FullPath {
		respondError(w, http.StatusBadRequest, "新名称与原名称相同")
		return
	}

	if _, err := os.Stat(newFullPath); err == nil {
		respondError(w, http.StatusConflict, "目标名称已存在")
		return
	}

	if err := os.Rename(rec.FullPath, newFullPath); err != nil {
		respondError(w, http.StatusInternalServerError, "重命名失败: "+err.Error())
		return
	}

	newExt := getExt(safeName)
	if err := UpdateFileRename(rec.FullPath, newFullPath, safeName, newExt); err != nil {
		log.Printf("[重命名] 更新数据库失败: %v", err)
	}

	if rec.IsDir {
		oldPrefix := rec.FullPath
		newPrefix := newFullPath
		if err := UpdateChildrenPaths(oldPrefix, newPrefix); err != nil {
			log.Printf("[重命名] 更新子文件路径失败: %v", err)
		}
	}

	log.Printf("[重命名] %s -> %s", rec.Name, safeName)
	respondJSON(w, http.StatusOK, nil)
}

// handleMove 文件移动接口
// POST /api/move {"uuid": "xxx", "target_path": "photos/2024"}
func handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID       string `json:"uuid"`
		TargetPath string `json:"target_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.UUID == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	targetAbsDir := toAbsPath(body.TargetPath)
	newFullPath := filepath.Join(targetAbsDir, rec.Name)

	// 校验新路径合法性
	newRelPath := filepath.Join(body.TargetPath, rec.Name)
	if _, err := sanitizePath(AppConfig.MountPath, newRelPath); err != nil {
		respondError(w, http.StatusBadRequest, "目标路径非法")
		return
	}

	// 不能移动到自身所在位置
	if newFullPath == rec.FullPath {
		respondError(w, http.StatusBadRequest, "目标位置与当前位置相同")
		return
	}

	// 检查目标位置是否存在同名文件
	if _, err := os.Stat(newFullPath); err == nil {
		respondError(w, http.StatusConflict, "目标位置已存在同名文件")
		return
	}

	// 确保目标目录存在
	if err := os.MkdirAll(targetAbsDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建目标目录失败")
		return
	}

	oldFullPath := rec.FullPath

	// 执行移动
	if err := os.Rename(oldFullPath, newFullPath); err != nil {
		respondError(w, http.StatusInternalServerError, "移动失败: "+err.Error())
		return
	}

	// 更新文件自身的数据库记录
	if err := UpdateFileMove(oldFullPath, newFullPath, targetAbsDir); err != nil {
		log.Printf("[移动] 更新数据库失败: %v", err)
	}

	// 如果是文件夹，更新所有子文件路径
	if rec.IsDir {
		if err := UpdateChildrenPaths(oldFullPath, newFullPath); err != nil {
			log.Printf("[移动] 更新子文件路径失败: %v", err)
		}
	}

	log.Printf("[移动] %s -> %s", toRelPath(oldFullPath), toRelPath(newFullPath))
	InvalidateDirSizeCache(rec.ParentPath)
	InvalidateDirSizeCache(targetAbsDir)
	respondJSON(w, http.StatusOK, nil)
}

// handleCopy 文件复制接口
// POST /api/copy {"uuid": "xxx", "target_path": "photos/2024"}
func handleCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID       string `json:"uuid"`
		TargetPath string `json:"target_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.UUID == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	// 校验目标路径合法性
	if _, err := sanitizePath(AppConfig.MountPath, body.TargetPath); err != nil {
		respondError(w, http.StatusBadRequest, "目标路径非法")
		return
	}

	targetAbsDir := toAbsPath(body.TargetPath)

	// 确保目标目录存在
	if err := os.MkdirAll(targetAbsDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建目标目录失败")
		return
	}

	// 处理同名文件冲突，自动加编号后缀
	dstPath := uniqueFilePath(targetAbsDir, rec.Name)

	if !rec.IsDir {
		// 复制普通文件
		if err := copyFile(rec.FullPath, dstPath); err != nil {
			respondError(w, http.StatusInternalServerError, "复制文件失败: "+err.Error())
			return
		}

		info, _ := os.Stat(dstPath)
		newRec := &FileRecord{
			UUID:       uuid.New().String(),
			Name:       filepath.Base(dstPath),
			ParentPath: targetAbsDir,
			FullPath:   dstPath,
			IsDir:      false,
			Size:       info.Size(),
			Ext:        getExt(filepath.Base(dstPath)),
		}
		UpsertFile(newRec)
		SubmitThumbnailTask(dstPath)
	} else {
		// 递归复制目录
		if err := copyDirRecursive(rec.FullPath, dstPath); err != nil {
			respondError(w, http.StatusInternalServerError, "复制目录失败: "+err.Error())
			return
		}

		// 为复制的根目录创建数据库记录
		newRec := &FileRecord{
			UUID:       uuid.New().String(),
			Name:       filepath.Base(dstPath),
			ParentPath: targetAbsDir,
			FullPath:   dstPath,
			IsDir:      true,
			Size:       0,
			Ext:        "",
		}
		UpsertFile(newRec)

		// 用 filepath.Walk 遍历复制的目录，为所有子项创建数据库记录并提交缩略图任务
		filepath.Walk(dstPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || path == dstPath {
				return nil
			}

			newUUID := uuid.New().String()
			fileRec := &FileRecord{
				UUID:       newUUID,
				Name:       info.Name(),
				ParentPath: filepath.Dir(path),
				FullPath:   path,
				IsDir:      info.IsDir(),
				Size:       info.Size(),
				Ext:        getExt(info.Name()),
			}
			UpsertFile(fileRec)

			if !info.IsDir() {
				SubmitThumbnailTask(path)
			}
			return nil
		})
	}

	log.Printf("[复制] %s -> %s", toRelPath(rec.FullPath), toRelPath(dstPath))
	respondJSON(w, http.StatusOK, nil)
}

// uniqueFilePath 在目录中生成不冲突的文件路径（同名时自动加编号后缀）
func uniqueFilePath(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return target
	}
	i := 1
	for {
		newName := fmt.Sprintf("%s(%d)%s", base, i, ext)
		target = filepath.Join(dir, newName)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
		i++
	}
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyDirRecursive 递归复制整个目录
func copyDirRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleStats 存储空间统计接口
// GET /api/stats
func handleStats(w http.ResponseWriter, r *http.Request) {
	var totalFiles, totalDirs, trashCount, totalSize int64

	// 使用单条条件聚合 SQL 替代 4 条串行查询
	err := DB.QueryRow(`
		SELECT
			COUNT(CASE WHEN is_dir=0 AND status='active' THEN 1 END),
			COUNT(CASE WHEN is_dir=1 AND status='active' THEN 1 END),
			COALESCE(SUM(CASE WHEN is_dir=0 AND status='active' THEN size END), 0),
			COUNT(CASE WHEN status='trashed' THEN 1 END)
		FROM files
	`).Scan(&totalFiles, &totalDirs, &totalSize, &trashCount)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "统计失败")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total_files": totalFiles,
		"total_dirs":  totalDirs,
		"total_size":  totalSize,
		"used_space":  totalSize,
		"trash_count": trashCount,
	})
}

// handleFileInfo 文件详情接口
// GET /api/file/info?uuid=xxx
func handleFileInfo(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	info := map[string]interface{}{
		"name":          rec.Name,
		"rel_path":      toRelPath(rec.FullPath),
		"is_dir":        rec.IsDir,
		"size":          rec.Size,
		"ext":           rec.Ext,
		"created_at":    rec.CreatedAt,
		"has_thumbnail": rec.HasThumbnail,
	}

	if rec.IsDir {
		var childCount int64
		DB.QueryRow(`SELECT COUNT(1) FROM files WHERE parent_path=? AND status='active'`, rec.FullPath).Scan(&childCount)
		info["child_count"] = childCount
	}

	respondJSON(w, http.StatusOK, info)
}

// handleRegenThumbnail 重新生成缩略图
// POST /api/file/regen-thumb {"uuid": "xxx"}
func handleRegenThumbnail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.UUID == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	thumbCache.Delete(rec.FullPath)
	SetThumbnail(rec.FullPath, 0)

	oldThumbPath := filepath.Join(CacheDir, rec.UUID+".jpg")
	os.Remove(oldThumbPath)

	SubmitThumbnailTask(rec.FullPath)

	log.Printf("[缩略图] 重新生成: %s", rec.Name)
	respondJSON(w, http.StatusOK, nil)
}

// handleVideoRepair 视频修复接口（重新编码以修复拖动进度条卡顿问题）
// POST /api/video/repair {"uuid": "xxx"}
func handleVideoRepair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.UUID == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if rec.IsDir {
		respondError(w, http.StatusBadRequest, "不能修复文件夹")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	if AppConfig.FFmpegPath == "" {
		respondError(w, http.StatusServiceUnavailable, "未配置 ffmpeg，无法修复视频")
		return
	}

	// 防止同一文件被并发修复
	if _, loaded := repairLocks.LoadOrStore(rec.FullPath, true); loaded {
		respondError(w, http.StatusConflict, "该视频正在修复中，请稍后再试")
		return
	}
	defer repairLocks.Delete(rec.FullPath)

	log.Printf("[视频修复] 开始修复: %s", rec.Name)

	// 生成唯一临时文件路径（避免并发冲突）
	tempPath := rec.FullPath + ".repair." + uuid.New().String()[:8] + ".mp4"

	// 构建 ffmpeg 修复命令：重新编码，添加密集关键帧（每 30 帧 ≈ 1 秒一个）
	args := []string{
		"-i", rec.FullPath,
		"-map_metadata", "0",
		"-map", "0:v",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-y",
		tempPath,
	}

	// 4 小时超时，防止 FFmpeg 卡死导致 goroutine 泄漏
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Hour)
	defer cancel()

	cmd := exec.CommandContext(ctx, AppConfig.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[视频修复] ffmpeg 执行失败: %v\n输出: %s", err, string(output))
		os.Remove(tempPath)
		DeleteFileRecord(tempPath) // 清理监听器可能已入库的临时文件记录
		if ctx.Err() != nil {
			respondError(w, http.StatusRequestTimeout, "视频修复超时，文件可能过大或损坏")
		} else {
			respondError(w, http.StatusInternalServerError, "视频修复失败，请检查视频文件是否完整")
		}
		return
	}

	// 获取修复后文件信息
	tempInfo, err := os.Stat(tempPath)
	if err != nil {
		os.Remove(tempPath)
		DeleteFileRecord(tempPath)
		respondError(w, http.StatusInternalServerError, "修复文件读取失败")
		return
	}

	// 用修复后的文件替换原文件
	originalPath := rec.FullPath
	backupPath := originalPath + ".backup." + uuid.New().String()[:8]

	// 先备份原文件
	if err := os.Rename(originalPath, backupPath); err != nil {
		os.Remove(tempPath)
		DeleteFileRecord(tempPath)
		log.Printf("[视频修复] 备份原文件失败: %v", err)
		respondError(w, http.StatusInternalServerError, "备份原文件失败")
		return
	}

	// 将修复后的文件移动到原位置
	if err := os.Rename(tempPath, originalPath); err != nil {
		// 恢复原文件
		os.Rename(backupPath, originalPath)
		os.Remove(tempPath)
		DeleteFileRecord(tempPath)
		log.Printf("[视频修复] 替换文件失败: %v", err)
		respondError(w, http.StatusInternalServerError, "替换文件失败，原文件未受影响")
		return
	}

	// 删除备份
	os.Remove(backupPath)
	DeleteFileRecord(backupPath)
	// 清理临时文件可能的数据库残留
	DeleteFileRecord(tempPath)

	// 更新数据库中的文件大小
	DB.Exec("UPDATE files SET size = ? WHERE uuid = ?", tempInfo.Size(), body.UUID)

	// 删除旧缩略图并重新生成
	thumbCache.Delete(originalPath)
	SetThumbnail(originalPath, 0)
	oldThumbPath := filepath.Join(CacheDir, rec.UUID+".jpg")
	os.Remove(oldThumbPath)
	SubmitThumbnailTask(originalPath)

	log.Printf("[视频修复] 完成: %s (大小: %s)", rec.Name, formatFileSize(tempInfo.Size()))
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"new_size": tempInfo.Size(),
	})
}

// ==================== 管理后台 API ====================

// handleAdminDashboard 仪表盘数据
// GET /api/admin/dashboard
func handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	var totalFiles, totalDirs, trashCount, thumbSuccess, thumbFailed, thumbPending, totalSize int64

	// 使用单条条件聚合 SQL 替代 7 条串行查询
	DB.QueryRow(`
		SELECT
			COUNT(CASE WHEN is_dir=0 AND status='active' THEN 1 END),
			COUNT(CASE WHEN is_dir=1 AND status='active' THEN 1 END),
			COUNT(CASE WHEN status='trashed' THEN 1 END),
			COUNT(CASE WHEN has_thumbnail=1 AND status='active' THEN 1 END),
			COUNT(CASE WHEN has_thumbnail=2 AND status='active' THEN 1 END),
			COUNT(CASE WHEN has_thumbnail=0 AND is_dir=0 AND status='active' THEN 1 END),
			COALESCE(SUM(CASE WHEN is_dir=0 AND status='active' THEN size END), 0)
		FROM files
	`).Scan(&totalFiles, &totalDirs, &trashCount, &thumbSuccess, &thumbFailed, &thumbPending, &totalSize)

	var thumbQueue int64
	if thumbWorker != nil {
		thumbQueue = int64(len(thumbWorker.taskChan))
	}

	uptime := time.Since(startTime).Truncate(time.Second)

	var diskTotal, diskUsed uint64
	if info, err := getDiskUsage(AppConfig.MountPath); err == nil {
		diskTotal = info.total
		diskUsed = info.used
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total_files":   totalFiles,
		"total_dirs":    totalDirs,
		"trash_count":   trashCount,
		"total_size":    totalSize,
		"thumb_success": thumbSuccess,
		"thumb_failed":  thumbFailed,
		"thumb_pending": thumbPending,
		"thumb_queue":   thumbQueue,
		"uptime":        uptime.String(),
		"disk_total":    diskTotal,
		"disk_used":     diskUsed,
	})
}

// handleAdminThumbFailed 获取缩略图生成失败的文件列表
// GET /api/admin/thumb/failed?page=1&page_size=50
func handleAdminThumbFailed(w http.ResponseWriter, r *http.Request) {
	page := 1
	pageSize := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 200 {
			pageSize = v
		}
	}

	var total int64
	DB.QueryRow(`SELECT COUNT(1) FROM files WHERE has_thumbnail=2 AND status='active'`).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := DB.Query(`
		SELECT uuid, name, full_path, size, ext, created_at
		FROM files WHERE has_thumbnail=2 AND status='active'
		ORDER BY created_at DESC LIMIT ? OFFSET ?
	`, pageSize, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	type FailedItem struct {
		UUID      string `json:"uuid"`
		Name      string `json:"name"`
		FullPath  string `json:"full_path"`
		RelPath   string `json:"rel_path"`
		Size      int64  `json:"size"`
		Ext       string `json:"ext"`
		CreatedAt string `json:"created_at"`
	}

	items := []FailedItem{}
	for rows.Next() {
		var item FailedItem
		if rows.Scan(&item.UUID, &item.Name, &item.FullPath, &item.Size, &item.Ext, &item.CreatedAt) == nil {
			item.RelPath = toRelPath(item.FullPath)
			items = append(items, item)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleAdminThumbRetryAll 重试所有失败的缩略图
// POST /api/admin/thumb/retry-all
func handleAdminThumbRetryAll(w http.ResponseWriter, r *http.Request) {
	rows, err := DB.Query(`SELECT full_path FROM files WHERE has_thumbnail=2 AND status='active'`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var fullPath string
		if rows.Scan(&fullPath) == nil {
			thumbCache.Delete(fullPath)
			SetThumbnail(fullPath, 0)
			SubmitThumbnailTask(fullPath)
			count++
		}
	}

	log.Printf("[管理] 批量重试缩略图: %d 个", count)
	respondJSON(w, http.StatusOK, map[string]int{"count": count})
}

// handleAdminThumbRetry 重试单个文件缩略图
// POST /api/admin/thumb/retry {"uuid": "xxx"}
func handleAdminThumbRetry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	rec, err := GetFileByUUID(body.UUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	thumbCache.Delete(rec.FullPath)
	SetThumbnail(rec.FullPath, 0)
	oldThumbPath := filepath.Join(CacheDir, rec.UUID+".jpg")
	os.Remove(oldThumbPath)
	SubmitThumbnailTask(rec.FullPath)

	respondJSON(w, http.StatusOK, nil)
}

// handleAdminThumbGeneratePending 批量补生成所有待处理的缩略图（has_thumbnail=0 的图片/视频文件）
// POST /api/admin/thumb/generate-pending
func handleAdminThumbGeneratePending(w http.ResponseWriter, r *http.Request) {
	extClauses := []string{}
	for ext := range imageExts {
		extClauses = append(extClauses, fmt.Sprintf("'%s'", ext))
	}
	for ext := range videoExts {
		extClauses = append(extClauses, fmt.Sprintf("'%s'", ext))
	}
	extIn := strings.Join(extClauses, ",")

	query := fmt.Sprintf(
		`SELECT full_path FROM files WHERE has_thumbnail=0 AND is_dir=0 AND status='active' AND ext IN (%s)`,
		extIn,
	)

	rows, err := DB.Query(query)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "查询失败")
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

	StartThumbnailWorkers()

	log.Printf("[管理] 补生成待处理缩略图: %d 个文件已加入队列", count)
	respondJSON(w, http.StatusOK, map[string]int{"count": count})
}

// handleAdminGetConfig 获取当前配置（脱敏）
// GET /api/admin/config
func handleAdminGetConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"port":            AppConfig.Port,
		"mount_path":      AppConfig.MountPath,
		"webdav_user":     AppConfig.WebdavUser,
		"has_webdav_pass": AppConfig.WebdavPass != "",
		"ffmpeg_path":     AppConfig.FFmpegPath,
		"auth_token":      maskToken(AppConfig.AuthToken),
		"has_auth":        AppConfig.AuthToken != "",
	})
}

// handleAdminSetConfig 更新配置
// POST /api/admin/config
func handleAdminSetConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port       int    `json:"port"`
		MountPath  string `json:"mount_path"`
		WebdavUser string `json:"webdav_user"`
		WebdavPass string `json:"webdav_pass"`
		FFmpegPath string `json:"ffmpeg_path"`
		AuthToken  string `json:"auth_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.WebdavUser != "" {
		AppConfig.WebdavUser = body.WebdavUser
	}
	if body.WebdavPass != "" {
		AppConfig.WebdavPass = body.WebdavPass
	}
	if body.FFmpegPath != "" {
		AppConfig.FFmpegPath = body.FFmpegPath
	}
	if body.AuthToken == "" || body.AuthToken == "CLEAR" {
		AppConfig.AuthToken = ""
	} else if body.AuthToken != maskToken(AppConfig.AuthToken) && body.AuthToken != "********" {
		AppConfig.AuthToken = body.AuthToken
	}

	// 保存到文件时对密码进行 base64 编码并加 b64: 前缀标记
	saveCfg := *AppConfig
	if saveCfg.WebdavPass != "" {
		saveCfg.WebdavPass = "b64:" + base64.StdEncoding.EncodeToString([]byte(saveCfg.WebdavPass))
	}

	configPath := filepath.Join(ExeDir, "config.yaml")
	data, _ := yaml.Marshal(&saveCfg)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		respondError(w, http.StatusInternalServerError, "保存配置失败")
		return
	}

	log.Printf("[管理] 配置已更新")
	respondJSON(w, http.StatusOK, nil)
}

// handleAdminLogs 获取最近日志
// GET /api/admin/logs?lines=100
func handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			lines = v
		}
	}

	logDir := filepath.Join(DataDir, "logs")
	logPath := filepath.Join(logDir, "pan.log")

	// 从文件尾部读取，避免加载整个日志文件到内存
	f, err := os.Open(logPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取日志失败")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取日志失败")
		return
	}

	// 从尾部逐块读取，找到足够的行数
	fileSize := stat.Size()
	bufSize := int64(8192)
	var result []string
	remaining := fileSize
	var leftover []byte

	for remaining > 0 && len(result) < lines+1 {
		readSize := bufSize
		if readSize > remaining {
			readSize = remaining
		}
		offset := remaining - readSize
		buf := make([]byte, readSize)
		_, err := f.ReadAt(buf, offset)
		if err != nil {
			break
		}

		// 将 leftover 拼接到当前块尾部
		if len(leftover) > 0 {
			buf = append(buf, leftover...)
			leftover = nil
		}

		// 按行分割
		chunk := strings.Split(string(buf), "\n")
		// 第一个元素可能是不完整行，留给下一轮
		leftover = []byte(chunk[0])
		for i := len(chunk) - 1; i >= 1; i-- {
			line := chunk[i]
			if line != "" || len(result) > 0 {
				result = append([]string{line}, result...)
			}
			if len(result) >= lines {
				break
			}
		}
		remaining = offset
	}

	// 如果还有剩余的第一行
	if remaining == 0 && len(leftover) > 0 && len(result) < lines {
		result = append([]string{string(leftover)}, result...)
	}

	// 限制最终行数
	if len(result) > lines {
		result = result[len(result)-lines:]
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"lines": result,
		"total": len(result),
	})
}

func maskToken(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "********"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

type diskInfo struct {
	total uint64
	used  uint64
}

// getDiskUsage 的平台实现见 diskusage_unix.go / diskusage_windows.go

// handleZipDownload 文件夹ZIP打包下载接口
// GET /api/download/zip?uuid=xxx
func handleZipDownload(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.URL.Query().Get("uuid")
	if uuidStr == "" {
		respondError(w, http.StatusBadRequest, "缺少 uuid 参数")
		return
	}

	rec, err := GetFileByUUID(uuidStr)
	if err != nil {
		respondError(w, http.StatusNotFound, "文件不存在")
		return
	}

	if _, err := os.Stat(rec.FullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "文件已被删除")
		return
	}

	if !rec.IsDir {
		// 非目录走普通下载
		w.Header().Set("Content-Disposition", contentDispositionName(rec.Name))
		w.Header().Set("Content-Type", detectContentType(rec.FullPath))
		http.ServeFile(w, r, rec.FullPath)
		return
	}

	log.Printf("[ZIP下载] 打包目录: %s (来自 %s)", rec.Name, r.RemoteAddr)

	zipName := rec.Name + ".zip"
	escapedZipName := url.PathEscape(zipName)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, escapedZipName, escapedZipName))
	w.Header().Set("Content-Transfer-Encoding", "binary")

	zipWriter := zipNewWriter(w)
	defer zipWriter.Close()

	err = filepath.WalkDir(rec.FullPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if isHiddenFile(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if isHiddenFile(d.Name()) {
			return nil
		}

		relToDir, err := filepath.Rel(rec.FullPath, path)
		if err != nil {
			return nil
		}
		zipPath := filepath.ToSlash(relToDir)

		err = zipWriter.AddFile(zipPath, path)
		if err != nil {
			log.Printf("[ZIP下载] 写入文件失败: %s, %v", path, err)
		}
		return nil
	})

	if err != nil {
		log.Printf("[ZIP下载] 遍历目录失败: %v", err)
	}
}

// ==================== 分片上传 ====================

// ChunkUploadMeta 分片上传元信息
type ChunkUploadMeta struct {
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	TotalChunks int    `json:"total_chunks"`
	Path        string `json:"path"`
}

// handleUploadChunkInit 初始化分片上传
// POST /api/upload/chunk {"file_name":"big.mp4","file_size":104857600,"total_chunks":10,"path":"videos"}
func handleUploadChunkInit(w http.ResponseWriter, r *http.Request) {
	var meta ChunkUploadMeta
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		respondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if meta.FileName == "" || meta.TotalChunks <= 0 {
		respondError(w, http.StatusBadRequest, "参数不完整")
		return
	}

	if meta.Path != "" {
		if _, err := sanitizePath(AppConfig.MountPath, meta.Path); err != nil {
			respondError(w, http.StatusBadRequest, "非法路径")
			return
		}
	}

	uploadId := uuid.New().String()

	metaPath := filepath.Join(DataDir, uploadId+".meta")
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		respondError(w, http.StatusInternalServerError, "创建上传任务失败")
		return
	}

	chunkDir := filepath.Join(DataDir, uploadId)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		os.Remove(metaPath)
		respondError(w, http.StatusInternalServerError, "创建临时目录失败")
		return
	}

	log.Printf("[上传] 分片上传初始化: %s (%d 片, %s)", meta.FileName, meta.TotalChunks, formatFileSize(meta.FileSize))
	respondJSON(w, http.StatusOK, map[string]string{"upload_id": uploadId})
}

// handleUploadChunk 上传单个分片
// POST /api/upload/chunk/{uploadId}  multipart: chunk_index, chunk
func handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	uploadId := mux.Vars(r)["uploadId"]

	metaPath := filepath.Join(DataDir, uploadId+".meta")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "上传任务不存在")
		return
	}

	var meta ChunkUploadMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		respondError(w, http.StatusInternalServerError, "读取上传信息失败")
		return
	}

	if err := r.ParseMultipartForm(0); err != nil {
		respondError(w, http.StatusBadRequest, "解析表单失败")
		return
	}

	chunkIndexStr := r.FormValue("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		respondError(w, http.StatusBadRequest, "分片索引无效")
		return
	}

	file, _, err := r.FormFile("chunk")
	if err != nil {
		respondError(w, http.StatusBadRequest, "缺少分片文件")
		return
	}
	defer file.Close()

	chunkPath := filepath.Join(DataDir, uploadId, fmt.Sprintf("chunk_%d", chunkIndex))
	dst, err := os.Create(chunkPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "保存分片失败")
		return
	}

	_, err = io.Copy(dst, file)
	dst.Close()
	if err != nil {
		os.Remove(chunkPath)
		respondError(w, http.StatusInternalServerError, "写入分片失败")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"chunk_index": chunkIndex,
		"received":    true,
	})
}

// handleUploadChunkComplete 完成分片上传，合并文件
// POST /api/upload/chunk/{uploadId}/complete
func handleUploadChunkComplete(w http.ResponseWriter, r *http.Request) {
	uploadId := mux.Vars(r)["uploadId"]

	metaPath := filepath.Join(DataDir, uploadId+".meta")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "上传任务不存在")
		return
	}

	var meta ChunkUploadMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		respondError(w, http.StatusInternalServerError, "读取上传信息失败")
		return
	}

	chunkDir := filepath.Join(DataDir, uploadId)

	// 直接检查磁盘上的分片文件是否存在
	received := 0
	for i := 0; i < meta.TotalChunks; i++ {
		if _, err := os.Stat(filepath.Join(chunkDir, fmt.Sprintf("chunk_%d", i))); err == nil {
			received++
		}
	}

	if received != meta.TotalChunks {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("分片不完整，已收到 %d/%d", received, meta.TotalChunks))
		return
	}

	absDir := toAbsPath(meta.Path)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建目标目录失败")
		return
	}

	safeName := filepath.Base(meta.FileName)

	// 合并到临时文件并计算 MD5
	tmpPath := filepath.Join(absDir, ".upload_chunk_"+uploadId)
	hash := md5.New()
	dstFile, err := os.Create(tmpPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "创建目标文件失败")
		return
	}

	var totalWritten int64
	for i := 0; i < meta.TotalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d", i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			dstFile.Close()
			os.Remove(tmpPath)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取分片 %d 失败", i))
			return
		}

		written, err := io.Copy(io.MultiWriter(dstFile, hash), chunkFile)
		chunkFile.Close()
		if err != nil {
			dstFile.Close()
			os.Remove(tmpPath)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("写入分片 %d 失败", i))
			return
		}
		totalWritten += written
	}
	dstFile.Close()

	fileMD5 := hex.EncodeToString(hash.Sum(nil))

	// 去重检查
	exactDup, contentDup := FindDuplicate(absDir, safeName, fileMD5, totalWritten)

	if exactDup != nil || contentDup != nil {
		os.RemoveAll(chunkDir)
		os.Remove(metaPath)
		os.Remove(tmpPath)
		existingName := safeName
		if contentDup != nil {
			existingName = contentDup.Name
		}
		log.Printf("[上传] 跳过内容重复: %s (已存在: %s)", safeName, existingName)
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"name":    safeName,
			"size":    totalWritten,
			"skipped": true,
		})
		return
	}

	// 同名冲突检查
	finalName := safeName
	dstPath := filepath.Join(absDir, safeName)
	renamed := false
	if _, err := os.Stat(dstPath); err == nil {
		dstPath = uniqueFilePath(absDir, safeName)
		finalName = filepath.Base(dstPath)
		renamed = true
	}

	os.Rename(tmpPath, dstPath)
	MarkUploading(dstPath)

	rec := &FileRecord{
		UUID:       uuid.New().String(),
		Name:       finalName,
		ParentPath: absDir,
		FullPath:   dstPath,
		IsDir:      false,
		Size:       totalWritten,
		Ext:        getExt(finalName),
		MD5:        fileMD5,
	}
	UpsertFile(rec)

	SubmitThumbnailTask(dstPath)
	ClearUploading(dstPath)

	os.RemoveAll(chunkDir)
	os.Remove(metaPath)

	log.Printf("[上传] 完成: %s (%s)%s", finalName, formatFileSize(totalWritten), map[bool]string{true: " (已重命名)", false: ""}[renamed])
	InvalidateDirSizeCache(absDir)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":    finalName,
		"size":    totalWritten,
		"renamed": renamed,
	})
}

// handleUploadChunkStatus 查询分片上传进度
// GET /api/upload/chunk/{uploadId}/status
func handleUploadChunkStatus(w http.ResponseWriter, r *http.Request) {
	uploadId := mux.Vars(r)["uploadId"]

	metaPath := filepath.Join(DataDir, uploadId+".meta")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "上传任务不存在")
		return
	}

	var meta ChunkUploadMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		respondError(w, http.StatusInternalServerError, "读取上传信息失败")
		return
	}

	chunkDir := filepath.Join(DataDir, uploadId)
	receivedIndices := []int{}
	for i := 0; i < meta.TotalChunks; i++ {
		if _, err := os.Stat(filepath.Join(chunkDir, fmt.Sprintf("chunk_%d", i))); err == nil {
			receivedIndices = append(receivedIndices, i)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"upload_id":        uploadId,
		"file_name":        meta.FileName,
		"file_size":        meta.FileSize,
		"total_chunks":     meta.TotalChunks,
		"received_chunks":  len(receivedIndices),
		"received_indices": receivedIndices,
		"path":             meta.Path,
	})
}
