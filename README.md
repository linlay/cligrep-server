# cligrep-server

## 1. 项目简介
`cligrep-server` 是 CLI Grep 的 Go 后端，提供 CLI 首页列表、详情、收藏、评论、受限执行、站内 builtin 命令，以及正式用户认证能力。

服务以宿主机原生进程运行，依赖 MySQL 持久化数据，依赖 Docker CLI 执行受限沙箱命令。服务源码只 seed 平台自有条目；内容型 CLI catalog 与 release 元数据应通过 admin/API 单独管理。

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
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u root -p < scripts/mysql/init.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/schema.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/seed-clis.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/seed-cli-locales.sql
```

说明：
- `scripts/mysql/init.sql` 只负责建库、建用户、授权。
- `scripts/mysql/schema.sql` 是唯一 schema 真相源。
- `scripts/mysql/seed-clis.sql` 只导入平台自有 CLI catalog。
- `scripts/mysql/seed-cli-locales.sql` 只导入平台自有 locale 文案。
- 通过 `mysql` CLI 手工导入时，必须显式传 `--default-character-set=utf8mb4`，否则中文 locale seed 可能以错误字符集写入数据库并产生乱码。

如需一次性导入平台 seed，推荐执行：

```bash
go run ./cmd/seed-catalog
```

内容型 CLI 不在初始化时内置。新环境如需导入内容型 CLI，请在服务启动后通过 admin/API 创建 CLI、发布状态和 release 元数据。

### 现有数据库升级到本次版本
执行一次：

```bash
mysql -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/migrate-20260327-cleanup.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/migrate-20260328-cli-locales.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/migrate-20260330-official-url.sql
mysql --default-character-set=utf8mb4 -h <db-host> -P <db-port> -u <db-user> -p < scripts/mysql/seed-cli-locales.sql
```

说明：
- `migrate-20260327-cleanup.sql` 会补 `cli_registry` 计数列、回填收藏/评论/运行计数，并删除 `seed_execution_record`。
- `migrate-20260328-cli-locales.sql` 会创建 `cli_locale_content`。
- `migrate-20260330-official-url.sql` 会把 `cli_registry` 的官方地址列从 `GITHUB_URL_` 迁移为 `OFFICIAL_URL_`，并回填旧数据。
- `seed-cli-locales.sql` 会回灌平台自有 locale 文案；如果先前通过未指定 `utf8mb4` 的 `mysql` CLI 导入过中文数据，也应按上面的方式重新执行一次覆盖修复。

### release 数据同步
在目标 CLI 已经通过 admin/API 创建并处于可见状态后执行：

```bash
go run ./cmd/release-sync
```

无参数时会从 `CLIGREP_RELEASES_ROOT` 的一级子目录自动发现候选 slug，并跳过没有匹配已发布 catalog 项的目录。

如果只同步指定 slug，可执行：

```bash
go run ./cmd/release-sync my-cli another-cli
```

`release-sync` 不会隐式创建 CLI catalog；显式指定的目标 CLI 不存在时会直接报错。

### 批量镜像脚本
可选运维脚本：

```bash
./scripts/import-upstream-clis.sh
```

默认行为：
- 抓取 `gh`、`playwright`、`vercel`、`supabase`、`ffmpeg`、`notebooklm` 的最近两个稳定版本。
- 先在本地 staging 目录整理成 `slug/vX.Y.Z` + `slug/latest` 结构。
- 同步到 `singapore02:/docker/cli-releases`。
- 不会写入 CLI catalog 或 release 数据库记录；这些内容需要后续通过 admin/API 和 `release-sync` 单独处理。

如需只做本地 staging 检查，可执行：

```bash
./scripts/import-upstream-clis.sh --dry-run --keep-stage
```

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
- `GET /api/v1/admin/clis`
- `POST /api/v1/admin/clis`
- `POST /api/v1/admin/clis/:slug/releases`
- `POST /api/v1/admin/clis/:slug/releases/:version/assets`

### 常见排查
- 启动失败且提示 `validate configuration`：检查 `.env` 中的 `CLIGREP_DB_*` 必填项。
- `/healthz` 返回 `sandboxReady=false`：检查 Docker daemon、BusyBox 镜像、Python 镜像。
- 页面无 CLI 数据：平台自有条目可通过 `go run ./cmd/seed-catalog` 导入；内容型 CLI 需要通过 admin/API 单独创建。
- release 数据为空：确认目标 CLI 已通过 admin/API 创建并发布，再执行 `go run ./cmd/release-sync`。
- 批量镜像后页面仍无 release：确认对应 CLI catalog 已存在，再执行 `go run ./cmd/release-sync <slug...>`。
- 现有库升级后计数异常：重新执行 `scripts/mysql/migrate-20260327-cleanup.sql` 回填计数。
- 中文 locale 出现乱码：检查导入命令是否显式带了 `--default-character-set=utf8mb4`，然后重新执行 `scripts/mysql/seed-cli-locales.sql` 覆盖修复。
