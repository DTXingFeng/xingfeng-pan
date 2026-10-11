# 刑风网盘 XingFeng Pan

> 单二进制内网网盘 · 自建私有云盘 · 局域网文件服务器
> Single-binary self-hosted cloud storage & file server for your LAN.

Go + Vue 3 + SQLite，无 CGO 依赖，编译一次扔到服务器即可运行。适合家用 NAS、家庭服务器或公司内网的自建网盘方案：下载一个可执行文件，指向硬盘目录，即得完整的网页端网盘 + WebDAV 挂载。

## 功能

- **文件管理**：目录浏览、排序、分页、模糊搜索（按类型过滤）、重命名、移动、复制、新建文件夹
- **上传**：普通上传、大文件分片上传（断点续传、状态查询）、MD5 秒传（同目录同内容去重）；实时进度与速度，完成/失败/重复跳过结果保留到手动清除
- **下载**：Range 断点续传、文件夹 ZIP 打包下载
- **在线预览**：图片/视频缩略图（视频缩略图依赖 ffmpeg）、视频流式播放与转码、损坏视频修复
- **回收站**：删除先进回收站，支持恢复 / 彻底删除 / 清空，30 天自动清理；磁盘上已被外部删除的“幽灵记录”可直接清除
- **WebDAV**：挂载为本地磁盘 / 第三方客户端直接读写
- **管理面板**：仪表盘（磁盘用量、文件统计）、缩略图失败重试、日志查看、在线修改配置
- **自同步**：启动全量扫描 + fsnotify 实时监听，磁盘与数据库保持一致，支持外部直接改动文件

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go（标准库 + gorilla/mux + fsnotify） |
| 数据库 | SQLite（modernc.org/sqlite，纯 Go 无 CGO，单文件零运维） |
| 前端 | Vue 3 + Element Plus + Vite，`go:embed` 打进二进制 |
| 可选依赖 | ffmpeg（视频缩略图 / 转码 / 修复；无 ffmpeg 时其余功能正常） |

## 快速开始

1. 下载或编译得到可执行文件（Linux/Windows 均可），放到任意目录
2. 首次运行自动生成 `config.yaml`，修改 `mount_path` 指向你的存储目录
3. 再次启动，浏览器访问 `http://内网IP:8080`

WebDAV 地址：`http://内网IP:8080/dav`（账号密码见 `config.yaml` 的 `webdav_user` / `webdav_pass`）

## 构建

后端（仓库内已含构建好的 `frontend/dist`，可直接编译）：

```bash
cd app
go build -ldflags='-s -w' -o pan .
```

交叉编译（例：在 Windows 上编译 Linux/arm64 服务器版本）：

```bash
$env:GOOS='linux'; $env:GOARCH='arm64'
go build -ldflags='-s -w' -o pan .
```

前端（仅改动前端时需要重新构建，产物在 `frontend/dist`）：

```bash
cd app/frontend
npm install
npm run build
```

运行测试：

```bash
cd app
go test ./...
```

前端上传与缓存回归测试：

```bash
cd app/frontend
npm test
# 浏览器验证（需 Python Playwright 和 Chromium，先运行 npm run build）
python tests/upload_browser_test.py
```

## 配置说明

| 字段 | 说明 |
|---|---|
| `port` | HTTP 服务端口，默认 8080 |
| `mount_path` | 网盘文件实际存储目录 |
| `webdav_user` / `webdav_pass` | WebDAV 账号密码（密码支持 bcrypt 哈希） |
| `ffmpeg_path` | ffmpeg 可执行文件路径，默认取 PATH |
| `auth_token` | API 直连令牌，留空不启用 |

完整示例见 [app/config.example.yaml](app/config.example.yaml)。

## 安全提示

本项目定位**内网自用**，未做 HTTPS / 防暴力破解等公网级防护：

- 首次部署务必修改 WebDAV 默认密码（`admin123`）
- 请勿直接暴露到公网；如需外网访问，建议套反代 + TLS + 访问控制

## 目录结构

```
app/            后端源码
  frontend/     Vue 3 前端源码（dist/ 为构建产物，嵌入二进制）
  config.example.yaml
test_files/     本地开发测试挂载目录（不入库）
data/           运行时自动生成：数据库 / 日志 / 缩略图缓存
```

## License

[MIT](LICENSE)
