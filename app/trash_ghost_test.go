package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// setupTrashTest 为每个用例初始化独立的临时挂载目录与数据库
func setupTrashTest(t *testing.T) string {
	t.Helper()
	mount := t.TempDir()
	prevDB := DB
	prevConfig := AppConfig
	prevDataDir := DataDir
	AppConfig = &Config{MountPath: mount}
	DataDir = t.TempDir()
	if err := InitDB(); err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	t.Cleanup(func() {
		DB.Close()
		DB = prevDB
		AppConfig = prevConfig
		DataDir = prevDataDir
	})
	return mount
}

func countRows(t *testing.T, path string) int {
	t.Helper()
	var count int
	if err := DB.QueryRow(`SELECT COUNT(1) FROM files WHERE full_path=?`, path).Scan(&count); err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	return count
}

// TestTrashMoveGhostFile 磁盘上已不存在的“幽灵”记录：
// 网页删除必须成功并彻底清除记录，而不是报 rename ENOENT（用户实际症状）
func TestTrashMoveGhostFile(t *testing.T) {
	mount := setupTrashTest(t)

	ghostPath := filepath.Join(mount, "Iwara - Blue archive stuff 1 for test.mp4")
	if err := UpsertFile(&FileRecord{
		UUID:       uuid.New().String(),
		Name:       "Iwara - Blue archive stuff 1 for test.mp4",
		ParentPath: mount,
		FullPath:   ghostPath,
		Ext:        "mp4",
	}); err != nil {
		t.Fatalf("写入幽灵记录失败: %v", err)
	}

	if err := TrashMove(ghostPath); err != nil {
		t.Fatalf("删除幽灵文件应成功，实际失败: %v", err)
	}

	if count := countRows(t, ghostPath); count != 0 {
		t.Fatalf("幽灵记录应被彻底清除，仍残留 %d 条", count)
	}
}

// TestTrashMoveGhostDir 幽灵目录（含子记录）同样应被递归清除
func TestTrashMoveGhostDir(t *testing.T) {
	mount := setupTrashTest(t)

	dirPath := filepath.Join(mount, "ghostdir")
	childPath := filepath.Join(dirPath, "child.mp4")
	if err := UpsertFile(&FileRecord{UUID: uuid.New().String(), Name: "ghostdir", ParentPath: mount, FullPath: dirPath, IsDir: true}); err != nil {
		t.Fatalf("写入幽灵目录记录失败: %v", err)
	}
	if err := UpsertFile(&FileRecord{UUID: uuid.New().String(), Name: "child.mp4", ParentPath: dirPath, FullPath: childPath, Ext: "mp4"}); err != nil {
		t.Fatalf("写入子记录失败: %v", err)
	}

	if err := TrashMove(dirPath); err != nil {
		t.Fatalf("删除幽灵目录应成功，实际失败: %v", err)
	}

	if count := countRows(t, dirPath); count != 0 {
		t.Fatalf("幽灵目录记录应被清除，仍残留 %d 条", count)
	}
	if count := countRows(t, childPath); count != 0 {
		t.Fatalf("幽灵目录子记录应被清除，仍残留 %d 条", count)
	}
}

// TestTrashMoveRealFileStillWorks 回归保护：正常文件仍然移入 .trash 并标记 trashed
func TestTrashMoveRealFileStillWorks(t *testing.T) {
	mount := setupTrashTest(t)

	fileUUID := uuid.New().String()
	realPath := filepath.Join(mount, "real.mp4")
	if err := os.WriteFile(realPath, []byte("data"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := UpsertFile(&FileRecord{
		UUID:       fileUUID,
		Name:       "real.mp4",
		ParentPath: mount,
		FullPath:   realPath,
		Ext:        "mp4",
	}); err != nil {
		t.Fatalf("写入记录失败: %v", err)
	}

	if err := TrashMove(realPath); err != nil {
		t.Fatalf("删除正常文件失败: %v", err)
	}

	trashPath := filepath.Join(mount, ".trash", "real_"+fileUUID+".mp4")
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("文件应已移动到回收站: %v", err)
	}
	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Fatalf("原文件应已不存在: %v", err)
	}

	var status string
	if err := DB.QueryRow(`SELECT status FROM files WHERE full_path=?`, realPath).Scan(&status); err != nil {
		t.Fatalf("查询状态失败: %v", err)
	}
	if status != "trashed" {
		t.Fatalf("记录状态应为 trashed，实际 %q", status)
	}
}

// TestHandleDeleteGhostFile 模拟用户在网页上删除幽灵文件：API 应返回 200
func TestHandleDeleteGhostFile(t *testing.T) {
	mount := setupTrashTest(t)

	ghostPath := filepath.Join(mount, "Iwara - Blue archive stuff 1 for test.mp4")
	ghostUUID := uuid.New().String()
	if err := UpsertFile(&FileRecord{
		UUID:       ghostUUID,
		Name:       "Iwara - Blue archive stuff 1 for test.mp4",
		ParentPath: mount,
		FullPath:   ghostPath,
		Ext:        "mp4",
	}); err != nil {
		t.Fatalf("写入幽灵记录失败: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"uuid": ghostUUID})
	req := httptest.NewRequest(http.MethodPost, "/api/delete", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handleDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("网页删除幽灵文件应返回 200，实际 %d: %s", w.Code, w.Body.String())
	}
	if count := countRows(t, ghostPath); count != 0 {
		t.Fatalf("幽灵记录应被彻底清除，仍残留 %d 条", count)
	}
}
