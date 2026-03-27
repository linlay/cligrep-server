# cligrep-server

## 1. 项目简介
`cligrep-server` 是 CLI Grep 的 Go 后端，提供 CLI 首页列表、详情、收藏、评论、受限执行、站内 builtin 命令，以及正式用户认证能力。

服务以宿主机原生进程运行，依赖 MySQL 持久化数据，依赖 Docker CLI 执行受限沙箱命令。CLI catalog 不再由服务启动时自动 seed，改为手工导入 SQL。

## 2. 快速开始
### 前置要求
- Go 1.26
- MySQL 8.0+
- Docker Engine 与 Docker CLI

### 本地启动
```bash
cp .env.example .env
./build.sh
./start.sh
```

服务默认监听 `http://127.0.0.1:11802`。

### 停止服务
```bash
./stop.sh
```

### 测试
```bash
go test ./...
```

## 3. 配置说明
全部运行配置来自 `.env`，公开模板见 `.env.example`。

关键配置：
- `CLIGREP_HTTP_ADDR`：HTTP 监听地址。
- `CLIGREP_DB_HOST` / `CLIGREP_DB_PORT` / `CLIGREP_DB_NAME` / `CLIGREP_DB_USER` / `CLIGREP_DB_PASSWORD`：MySQL 连接配置。
- `CLIGREP_BUSYBOX_IMAGE` / `CLIGREP_PYTHON_IMAGE`：沙箱镜像。
- `CLIGREP_CONTAINER_CPUS` / `CLIGREP_CONTAINER_MEMORY` / `CLIGREP_COMMAND_TIMEOUT_MS`：沙箱资源与超时限制。
- `CLIGREP_CORS_ORIGIN`：允许跨域来源。
- `CLIGREP_AUTH_GOOGLE_CLIENT_ID` / `CLIGREP_AUTH_GOOGLE_CLIENT_SECRET` / `CLIGREP_AUTH_GOOGLE_REDIRECT_URL`：Google OAuth 配置。
- `CLIGREP_AUTH_GOOGLE_SUCCESS_URL` / `CLIGREP_AUTH_GOOGLE_FAILURE_URL`：Google 登录回跳地址。
- `CLIGREP_AUTH_SESSION_TTL_HOURS`：站内 session TTL。
- `CLIGREP_AUTH_COOKIE_NAME` / `CLIGREP_AUTH_COOKIE_SECURE` / `CLIGREP_AUTH_COOKIE_DOMAIN` / `CLIGREP_AUTH_COOKIE_SAMESITE`：登录态 Cookie 配置。
- `CLIGREP_RELEASES_ROOT` / `CLIGREP_RELEASES_BASE_URL`：release 目录和下载地址。

## 4. 部署
### 全新数据库初始化
按顺序执行一次：

```bash
mysql -h <db-host> -P <db-port> -u root -p < scripts/mysql/init.sql
mysql -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/schema.sql
mysql -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/seed-clis.sql
```

说明：
- `scripts/mysql/init.sql` 只负责建库、建用户、授权。
- `scripts/mysql/schema.sql` 是唯一 schema 真相源。
- `scripts/mysql/seed-clis.sql` 负责导入基础 CLI catalog。

### 现有数据库升级到本次版本
执行一次：

```bash
mysql -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/migrate-20260327-cleanup.sql
```

这个迁移会补 `cli_registry` 计数列、回填收藏/评论/运行计数，并删除 `seed_execution_record`。

### release 数据同步
在 CLI catalog 已导入后执行：

```bash
go run ./cmd/release-sync
```

如果指定 slug，可执行：

```bash
go run ./cmd/release-sync dbx httpx
```

`release-sync` 不再隐式导入 CLI catalog；目标 CLI 不存在时会直接报错。

### 前端联调
- 前端通过 `/api` 和 `/healthz` 访问本服务。
- 站内认证使用 Google OAuth、本地密码和 HttpOnly session。
- 如需联调 Google 登录，`.env` 中的 `CLIGREP_AUTH_GOOGLE_REDIRECT_URL` 必须与 Google 控制台登记值完全一致。

## 5. 运维
### 常用接口
- `GET /healthz`
- `GET /api/v1/clis/trending`
- `GET /api/v1/clis/:slug`
- `POST /api/v1/exec`
- `POST /api/v1/builtin/exec`
- `GET /api/v1/auth/google/start`
- `GET /api/v1/auth/google/callback`
- `POST /api/v1/auth/local/register`
- `POST /api/v1/auth/local/login`
- `GET /api/v1/auth/me`
- `PATCH /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `GET /api/v1/favorites`
- `POST /api/v1/favorites`
- `GET /api/v1/comments`
- `POST /api/v1/comments`

### 常见排查
- 启动失败且提示 `validate configuration`：检查 `.env` 中的 `CLIGREP_DB_*` 必填项。
- `/healthz` 返回 `sandboxReady=false`：检查 Docker daemon、BusyBox 镜像、Python 镜像。
- 页面无 CLI 数据：确认已经执行 `scripts/mysql/seed-clis.sql`。
- release 数据为空：确认已经先导入 catalog，再执行 `go run ./cmd/release-sync`。
- 现有库升级后计数异常：重新执行 `scripts/mysql/migrate-20260327-cleanup.sql` 回填计数。
