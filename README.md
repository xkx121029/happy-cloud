# Happy-Cloud ☁️ 跨平台私有云盘

Happy-Cloud 是一个支持多端访问的私有云盘系统，提供文件上传、存储、管理、分享能力，包含用户前台与管理后台，后端服务容器化部署，支持 ARM64 架构（可部署至 NAS 等 ARM 设备）。

## ✨ 功能特性

- **多端访问**：Web（浏览器）、Android（App）、Windows（桌面）三端共用同一套 API
- **文件管理**：上传 / 下载 / 重命名 / 移动 / 删除 / 新建文件夹，网格与列表双视图
- **高效传输**：秒传（SHA256 去重）、大文件分片上传（每片 10MB，并发）、断点续传
- **文件分享**：生成分享链接，支持访问密码与有效期
- **存储配额**：按用户配额管理存储空间，实时展示用量
- **管理后台**：用户管理（禁用 / 配额）、全站文件检索、系统监控、操作日志审计
- **容器化部署**：Docker Compose 一键启动，支持 ARM64

## 🏗️ 项目结构

```
happy-cloud/
├── backend/          # Go + Gin 后端服务（用户/文件/分享/管理模块）
├── web/              # Vue3 + TypeScript Web 端（用户前台 + 管理后台）
├── android/          # Android 客户端（Kotlin + Jetpack Compose）
├── windows/          # Windows 桌面客户端（Flutter）
├── docs/
│   └── api.md        # API 契约文档
└── docker-compose.yml
```

## 🔧 技术栈

| 端 | 技术 |
|---|---|
| 后端 | Go + Gin + GORM + MySQL + JWT |
| Web | Vue3 + TypeScript + Vite + Pinia |
| Android | Kotlin + Jetpack Compose + Material 3 Expressive |
| Windows | Flutter Desktop |
| 存储 | 本地磁盘（可选 MinIO / 云 OSS） |
| 部署 | Docker Compose（支持 arm64） |

## 🚀 快速开始

### 方式一：Docker Compose（推荐）

```bash
docker compose up -d --build
```

- Web 端：http://localhost
- 管理后台：http://localhost/admin
- 后端 API：http://localhost:8080/api

> 注册的第一个 `admin` 用户即为管理员（可用 `ADMIN_USER` 环境变量指定）。

### 方式二：本地开发

**后端**

```bash
cd backend
go mod tidy
go run main.go
# 环境变量示例
# DB_DSN=root:123456@tcp(127.0.0.1:3306)/happy_cloud?charset=utf8mb4&parseTime=True&loc=Local
# STORAGE_PATH=./data
```

**Web**

```bash
cd web
npm install
npm run dev
```

## 📚 API 文档

详见 [docs/api.md](docs/api.md)，统一响应格式 `{ code, message, data }`，鉴权使用 JWT Bearer Token（有效期 7 天）。

## 🐳 部署说明

1. 服务器安装 Docker 与 Docker Compose
2. 构建镜像时指定 `TARGETARCH`（默认 amd64，ARM 设备设为 `arm64`）
3. `docker compose up -d` 启动
4. 通过 Nginx 反向代理对外暴露 443（HTTPS 证书），管理后台走 `/admin` 子路径

## 📄 License

[MIT](LICENSE)
