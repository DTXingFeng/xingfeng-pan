//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

// getDiskUsage 查询路径所在磁盘的用量（Windows），使本项目可在 Windows 上构建和测试
func getDiskUsage(path string) (*diskInfo, error) {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var free, total uint64
	if err := windows.GetDiskFreeSpaceEx(p16, &free, &total, nil); err != nil {
		return nil, err
	}
	return &diskInfo{total: total, used: total - free}, nil
}
