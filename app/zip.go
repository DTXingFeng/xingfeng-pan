package main

import (
	"archive/zip"
	"io"
	"os"
)

// ZipWriter ZIP 写入器封装
type ZipWriter struct {
	writer *zip.Writer
}

// zipNewWriter 创建 ZIP 写入器
func zipNewWriter(w io.Writer) *ZipWriter {
	return &ZipWriter{
		writer: zip.NewWriter(w),
	}
}

// Close 关闭 ZIP 写入器
func (z *ZipWriter) Close() error {
	return z.writer.Close()
}

// AddFile 向 ZIP 中添加一个文件
func (z *ZipWriter) AddFile(zipPath, filePath string) error {
	header := &zip.FileHeader{
		Name:   zipPath,
		Method: zip.Deflate,
	}

	if needsUTF8(zipPath) {
		header.Name = zipPath
		header.SetMode(0644)
		header.NonUTF8 = false
	}

	fw, err := z.writer.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(fw, file)
	return err
}

// needsUTF8 判断文件名是否需要 UTF-8 标志
func needsUTF8(name string) bool {
	for _, r := range name {
		if r > 127 {
			return true
		}
	}
	return false
}
