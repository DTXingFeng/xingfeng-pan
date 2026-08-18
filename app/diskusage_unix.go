//go:build !windows

package main

import "syscall"

// getDiskUsage 查询路径所在文件系统的磁盘用量（Unix/Linux）
func getDiskUsage(path string) (*diskInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return &diskInfo{total: total, used: total - free}, nil
}
