package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构体
type Config struct {
	Port       int    `yaml:"port"`
	MountPath  string `yaml:"mount_path"`
	WebdavUser string `yaml:"webdav_user"`
	WebdavPass string `yaml:"webdav_pass"`
	FFmpegPath string `yaml:"ffmpeg_path"`
	AuthToken  string `yaml:"auth_token"`
}

// 全局配置实例
var AppConfig *Config

// 程序所在目录（运行时确定）
var ExeDir string

// 数据目录路径
var DataDir string
var CacheDir string

// 默认配置
func defaultConfig() *Config {
	// 跨平台默认挂载目录：用户主目录/XingFengFiles
	homeDir, _ := os.UserHomeDir()
	defaultMount := filepath.Join(homeDir, "XingFengFiles")

	return &Config{
		Port:       8080,
		MountPath:  defaultMount,
		WebdavUser: "admin",
		WebdavPass: "admin123",
		FFmpegPath: "ffmpeg",
	}
}

// InitConfig 初始化配置：读取或生成 config.yaml
func InitConfig() error {
	// 获取可执行文件所在目录
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}
	ExeDir = filepath.Dir(exePath)

	configPath := filepath.Join(ExeDir, "config.yaml")

	// 如果配置文件不存在，生成默认配置并退出
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := defaultConfig()
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("序列化默认配置失败: %w", err)
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("写入默认配置失败: %w", err)
		}
		fmt.Println("============================================")
		fmt.Println("已生成默认配置文件: config.yaml")
		fmt.Printf("配置文件路径: %s\n", configPath)
		fmt.Printf("默认挂载目录: %s\n", cfg.MountPath)
		fmt.Println("请通过管理面板修改挂载目录等设置")
		fmt.Println("============================================")
		AppConfig = cfg
		if _, err := os.Stat(cfg.MountPath); os.IsNotExist(err) {
			os.MkdirAll(cfg.MountPath, 0755)
		}
		log.Printf("[配置] 使用默认配置 (挂载目录: %s, 端口: %d)", cfg.MountPath, cfg.Port)
		return nil
	}

	// 读取现有配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 校验必填项
	if cfg.MountPath == "" {
		return fmt.Errorf("配置错误: mount_path 不能为空")
	}

	// 检查挂载目录是否存在，不存在则自动创建
	info, err := os.Stat(cfg.MountPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.MountPath, 0755); err != nil {
			return fmt.Errorf("创建挂载目录失败: %w", err)
		}
		log.Printf("[配置] 已自动创建挂载目录: %s", cfg.MountPath)
	} else if err != nil {
		return fmt.Errorf("配置错误: 无法访问 mount_path: %s, %v", cfg.MountPath, err)
	} else if !info.IsDir() {
		return fmt.Errorf("配置错误: mount_path 不是目录: %s", cfg.MountPath)
	}

	AppConfig = cfg

	// 解密 WebDAV 密码（配置文件中以 "b64:" 前缀标记 base64 编码）
	if cfg.WebdavPass != "" && strings.HasPrefix(cfg.WebdavPass, "b64:") {
		if decoded, err := base64.StdEncoding.DecodeString(cfg.WebdavPass[4:]); err == nil {
			cfg.WebdavPass = string(decoded)
		}
	}

	log.Printf("[配置] 操作系统: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("[配置] 端口: %d", cfg.Port)
	log.Printf("[配置] 挂载目录: %s", cfg.MountPath)
	log.Printf("[配置] FFmpeg路径: %s", cfg.FFmpegPath)
	return nil
}

// InitDirectories 初始化数据目录结构
func InitDirectories() error {
	DataDir = filepath.Join(ExeDir, "data")
	CacheDir = filepath.Join(DataDir, ".cache")

	dirs := []string{DataDir, CacheDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	log.Printf("[初始化] 数据目录: %s", DataDir)
	log.Printf("[初始化] 缓存目录: %s", CacheDir)
	return nil
}
