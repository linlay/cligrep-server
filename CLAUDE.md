# CLAUDE.md

## 1. 项目概览
`cligrep-server` 是 CLI Grep 的后端服务，负责 CLI 数据初始化、搜索与详情查询、评论与收藏持久化、Mock 用户会话，以及受限命令执行。

该项目使用 MySQL 作为主存储，使用 Docker 运行 BusyBox/Python 沙箱，但服务进程本身以原生 Go 应用运行。

## 2. 技术栈
- Go 1.26
- `net/http`
- MySQL（`github.com/go-sql-driver/mysql`）
- Docker CLI（用于沙箱执行）
- Shell 脚本：`build.sh`、`start.sh`、`stop.sh`

## 3. 架构设计
- `cmd/server/main.go` 负责加载配置、启动 HTTP 服务、处理优雅退出。
- `internal/config` 管理环境变量配置读取。
- `internal/api` 提供 HTTP 路由、序列化与 CORS 中间件。
- `internal/app` 编排业务流程，聚合 store、sandbox runner 与 builtin service。
- `internal/data` 负责 MySQL schema 初始化、查询与写入。
- `internal/sandbox` 使用宿主机 Docker CLI 在受限镜像中执行命令。
- `internal/seed` 负责初始化 CLI 列表、Mock 用户、收藏与执行记录种子数据。

## 4. 目录结构
- `cmd/server/`：服务启动入口。
- `internal/api/`：HTTP handler 与接口层逻辑。
- `internal/app/`：应用服务层。
- `internal/config/`：配置加载。
- `internal/data/`：MySQL 访问与 schema 管理。
- `internal/models/`：接口 DTO 与领域模型。
- `internal/sandbox/`：BusyBox / Python 沙箱执行逻辑。
- `internal/builtin/`：内置命令逻辑。
- `scripts/mysql/init.sql`：MySQL 建库、授权和建表脚本。
- `build.sh` / `start.sh` / `stop.sh`：本机部署生命周期脚本。

## 5. 数据结构
- `cli_registry`：CLI 元信息、展示属性、执行能力、运行环境和来源信息。
- `auth_user`：匿名用户和 mock 登录用户。
- `user_favorite`：用户与 CLI 的收藏关系。
- `user_comment`：CLI 评论内容。
- `sandbox_execution_log`：执行历史、stdout/stderr、退出码与耗时。
- `sandbox_generated_asset`：内置命令生成的脚本、Dockerfile、sandbox recipe 等内容。
- `seed_execution_record`：种子执行日志去重标记。
- 所有字段统一使用大写蛇形并以下划线结尾，例如 `USER_ID_`、`CREATED_AT_`、`DISPLAY_NAME_`。
- `models` 中定义了 `CLI`、`ExecutionResult`、`BuiltinExecResponse`、`Comment`、`FavoriteMutation`、`LoginRequest` 等接口数据结构。

## 6. API 定义
- `GET /healthz`：返回服务健康状态、镜像配置、MySQL 连接目标和运行能力摘要，并附带 `sandboxReady` 与 `sandbox` 详细探测结果。
- `GET /api/v1/clis/trending`：返回首页热门 CLI 列表。
- `GET /api/v1/clis/search`：按关键字搜索 CLI。
- `GET /api/v1/clis/:slug`：返回单个 CLI 详情、评论与示例命令。
- `POST /api/v1/exec`：执行沙箱 CLI。
- `POST /api/v1/builtin/exec`：执行网站内置命令。
- `POST /api/v1/auth/mock/anonymous`：创建匿名会话。
- `POST /api/v1/auth/mock/login`：按用户名创建 mock 会话。
- `POST /api/v1/auth/mock/logout`：结束 mock 会话。
- `GET/POST /api/v1/favorites`：读取或写入收藏。
- `GET/POST /api/v1/comments`：读取或新增评论。

## 7. 开发要点
- 配置全部来自环境变量，`.env.example` 是契约文件，`.env` 是本地真实值。
- `CLIGREP_CORS_ORIGIN` 现在直接作用于 HTTP 中间件，支持 `*` 或逗号分隔多个 origin。
- 数据库配置使用 `CLIGREP_DB_HOST`、`CLIGREP_DB_PORT`、`CLIGREP_DB_NAME`、`CLIGREP_DB_USER`、`CLIGREP_DB_PASSWORD`。
- 应用启动时会尝试创建 `cligrep` 数据库并自动初始化 MySQL 表结构；如账号缺少建库权限，先执行 `scripts/mysql/init.sql`。
- 沙箱执行依赖宿主机 Docker，服务本身不容器化。
- 服务启动时会主动探测 Docker CLI、Docker daemon、BusyBox 镜像、Python 镜像；若未就绪，只打印一次 warning，不阻止 HTTP 服务启动。
- 运行脚本以仓库内目录为默认目标：`build/`、`logs/`、`run/`。

## 8. 开发流程
- 初始化配置：`cp .env.example .env`
- 执行测试：`go test ./...`
- 构建二进制：`./build.sh`
- 启动服务：`./start.sh`
- 停止服务：`./stop.sh`
- 联调前端：启动本服务后，再启动 `cligrep-website` 的 Vite 或容器前端。

## 9. 已知约束与注意事项
- 运行环境必须具备 Docker CLI 与对应镜像，否则沙箱能力不可用。
- 当前鉴权是 mock 模式，不适用于正式生产认证场景。
- 当前 schema 依赖 MySQL 8.0+ 的 `JSON`、`utf8mb4` 与 `ON DUPLICATE KEY UPDATE` 能力。
- `start.sh` 默认用后台进程模式启动，适合轻量部署与联调；更重的生产托管可改用 systemd。

## 10. 沙箱排障
- 先看启动日志中的 `warning: sandbox is not ready`，再用 `GET /healthz` 确认 `sandbox.issues`。
- 常用命令：
  - `docker info`
  - `docker image inspect busybox:1.36.1`
  - `docker image inspect python:3.12-slim`
  - `docker pull busybox:1.36.1`
  - `docker pull python:3.12-slim`

## 11. 数据库排障
- 先确认 `.env` 中 MySQL 配置与实际一致，再执行 `scripts/mysql/init.sql` 建库建表。
- 常用命令：
  - `mysql -h 13.212.113.109 -P 3306 -u cligrep -p -e "SHOW DATABASES;"`
  - `mysql -h 13.212.113.109 -P 3306 -u cligrep -p -D cligrep -e "SHOW TABLES;"`
  - `mysql -h 13.212.113.109 -P 3306 -u cligrep -p -D cligrep -e "SHOW CREATE TABLE cli_registry\G"`
