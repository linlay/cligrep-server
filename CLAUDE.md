# CLAUDE.md

## 1. 项目概览
`cligrep-server` 是 CLI Grep 的后端服务，负责：
- CLI 首页列表与详情查询
- 收藏、评论、执行记录持久化
- 站内 builtin 命令执行
- Google OAuth、本地密码、HttpOnly session 用户认证
- release 目录同步到数据库

项目使用 MySQL 作为唯一持久化存储，使用 Docker CLI 驱动 BusyBox / Python 沙箱。服务源码只 seed 平台自有 catalog，内容型 CLI 通过 admin/API 单独管理。

## 2. 技术栈
- Go 1.26
- `net/http`
- MySQL 8.0+
- `github.com/go-sql-driver/mysql`
- Docker CLI
- Shell 脚本：`build.sh`、`start.sh`、`stop.sh`

## 3. 架构设计
- `cmd/server/main.go`：加载配置，初始化应用，启动 HTTP 服务。
- `cmd/release-sync/main.go`：扫描 release 目录并把版本与资产写入 MySQL。
- `internal/api`：HTTP 路由、序列化、CORS、中间件、session cookie 处理。
- `internal/app`：业务编排层，组合 store、sandbox runner、builtin service、Google OAuth provider。
- `internal/data`：MySQL schema 初始化、认证、查询、写入、计数维护。
- `internal/builtin`：站内 `grep`、`create`、`make` 命令。
- `internal/sandbox`：通过宿主机 Docker CLI 执行受限命令。
- `internal/releasesync`：读取 `CLIGREP_RELEASES_ROOT` 并 upsert `cli_release` / `cli_release_asset`。
- `internal/util`：命令拆分和输入约束辅助函数。
- `scripts/mysql`：数据库初始化、schema、catalog seed、一次性迁移 SQL，以及嵌入到 Go 的 schema 文件。

## 4. 目录结构
- `cmd/server/`：主服务入口。
- `cmd/release-sync/`：release 同步入口。
- `internal/api/`：HTTP handler 和请求上下文逻辑。
- `internal/app/`：应用服务层。
- `internal/builtin/`：站内 builtin 命令。
- `internal/config/`：环境变量配置读取与校验。
- `internal/data/`：MySQL store、认证与 schema 初始化。
- `internal/models/`：DTO 与领域模型。
- `internal/releasesync/`：release 目录扫描与同步。
- `internal/sandbox/`：BusyBox / Python 沙箱执行。
- `internal/util/`：命令与字符串辅助工具。
- `scripts/mysql/`：`init.sql`、`schema.sql`、`seed-clis.sql`、`seed-cli-locales.sql`、迁移 SQL，与嵌入 schema 的 Go 文件。

## 5. 数据结构
- `cli_registry`：CLI 主表，包含展示属性、运行属性、来源信息，以及 `FAVORITE_COUNT_`、`COMMENT_COUNT_`、`RUN_COUNT_` 三个聚合计数列。
- `auth_user`：站内用户表，支持 `local` 与 `google` provider。
- `auth_local_credential`：本地密码哈希。
- `auth_session`：session token hash、过期时间、访问信息。
- `auth_login_log`：认证成功/失败日志。
- `user_favorite`：用户收藏关系。
- `user_comment`：CLI 评论。
- `sandbox_execution_log`：CLI / builtin 执行日志。
- `sandbox_generated_asset`：builtin `create` / `make` 生成的内容。
- `cli_release`：CLI 版本记录。
- `cli_release_asset`：release 资产记录。
- 所有表字段使用大写蛇形并以下划线结尾，例如 `USER_ID_`、`CREATED_AT_`、`DISPLAY_NAME_`。

## 6. API 定义
- `GET /healthz`：服务健康状态、数据库连接信息、sandbox 探测结果、Google 配置摘要。
- `GET /api/v1/clis/trending`：首页 CLI 列表，支持排序模式。
- `GET /api/v1/clis/:slug`：单个 CLI 详情、评论、release、示例命令。
- `POST /api/v1/exec`：执行普通 CLI 沙箱命令。
- `POST /api/v1/builtin/exec`：执行站内 builtin 命令。
- `GET /api/v1/auth/google/start`：发起 Google OAuth。
- `GET /api/v1/auth/google/callback`：Google OAuth 回调。
- `POST /api/v1/auth/local/register`：本地账号注册并创建 session。
- `POST /api/v1/auth/local/login`：本地账号登录并创建 session。
- `GET /api/v1/auth/me`：读取当前 session 用户。
- `PATCH /api/v1/auth/me`：更新当前用户显示名。
- `POST /api/v1/auth/logout`：删除当前 session。
- `GET /api/v1/favorites` / `POST /api/v1/favorites`：读取或变更收藏。
- `GET /api/v1/comments` / `POST /api/v1/comments`：读取或新增评论。
- `GET /api/v1/admin/clis` / `POST /api/v1/admin/clis`：管理 CLI catalog。
- `POST /api/v1/admin/clis/:slug/releases`：创建 release 元数据。
- `POST /api/v1/admin/clis/:slug/releases/:version/assets`：上传 release 资产元数据。

已删除的旧接口：
- `GET /api/v1/clis/search`
- `POST /api/v1/auth/mock/anonymous`
- `POST /api/v1/auth/mock/login`
- `POST /api/v1/auth/mock/logout`

## 7. 开发要点
- 配置全部来自环境变量；`.env.example` 是公开模板，`.env` 是本地真实值。
- `scripts/mysql/schema.sql` 是唯一 schema 真相源，Go 启动时通过嵌入 SQL 执行建表语句。
- 新环境初始化顺序固定为：`init.sql` -> `schema.sql` -> `seed-clis.sql` -> `seed-cli-locales.sql`。
- `seed-clis.sql` / `seed-cli-locales.sql` 只覆盖平台自有条目，不再承载内容型 CLI catalog。
- 通过 `mysql` CLI 手工导入 SQL 时，必须显式带 `--default-character-set=utf8mb4`，否则中文 locale seed 可能被写成乱码。
- 现有环境升级到本次版本时，需要先执行 `migrate-20260327-cleanup.sql`，再部署新代码。
- locale 表接入版本还需要执行 `migrate-20260328-cli-locales.sql`，并重新导入 `seed-cli-locales.sql`。
- 内容型 CLI 应通过 admin/API 创建、发布，再由 `release-sync` 同步 release 元数据。
- `release-sync` 只同步 release 元数据，不负责导入 CLI catalog。
- `scripts/import-upstream-clis.sh` 是可选运维脚本，只负责镜像 release 文件，不负责写 catalog 或 release 表。
- `cli_registry` 的收藏、评论、运行计数在写路径同步维护，首页/详情/搜索直接读取计数列。
- 服务启动时会探测 Docker CLI、Docker daemon、BusyBox 镜像、Python 镜像；sandbox 未就绪不会阻止 HTTP 服务启动。

## 8. 开发流程
- 初始化配置：`cp .env.example .env`
- 初始化全新数据库：
  - `mysql --default-character-set=utf8mb4 -u root -p < scripts/mysql/init.sql`
  - `mysql --default-character-set=utf8mb4 -u <db-user> -p < scripts/mysql/schema.sql`
  - `mysql --default-character-set=utf8mb4 -u <db-user> -p < scripts/mysql/seed-clis.sql`
  - `mysql --default-character-set=utf8mb4 -u <db-user> -p < scripts/mysql/seed-cli-locales.sql`
  - 或直接执行 `go run ./cmd/seed-catalog`
- 内容型 CLI 初始化：
  - 通过 `/api/v1/admin/clis` 与 release 相关 admin 接口单独创建。
- 现有数据库升级：
  - `mysql -u <db-user> -p < scripts/mysql/migrate-20260327-cleanup.sql`
  - `mysql --default-character-set=utf8mb4 -u <db-user> -p < scripts/mysql/migrate-20260328-cli-locales.sql`
  - `mysql --default-character-set=utf8mb4 -u <db-user> -p < scripts/mysql/seed-cli-locales.sql`
- 执行测试：`go test ./...`
- 构建二进制：`./build.sh`
- 启动服务：`./start.sh`
- 停止服务：`./stop.sh`
- 同步 release：`go run ./cmd/release-sync`
- 镜像外部 release 文件：`./scripts/import-upstream-clis.sh`

## 9. 已知约束与注意事项
- 运行环境必须具备 Docker CLI 与对应镜像，否则沙箱能力不可用。
- 新代码默认只支持已经处于正式 auth/release 结构的数据库；不再保留运行时 schema upgrade 兼容层。
- `schema.sql` 只用于新库建表，不能替代现有库迁移，因为 `CREATE TABLE IF NOT EXISTS` 不会补已有表的缺失列。
- 如果未执行 `seed-clis.sql` / `seed-cli-locales.sql`，平台自有条目与 locale 内容不会完整。
- 当前 schema 依赖 MySQL 8.0+ 的 `JSON`、`utf8mb4` 与 `ON DUPLICATE KEY UPDATE`；一次性迁移脚本通过 `information_schema` + 动态 SQL 兼容不支持 `ADD COLUMN IF NOT EXISTS` 的实例。
- 如果中文 API 返回出现 `åŒ…ç®¡...` 一类乱码，优先检查手工导入时是否遗漏 `--default-character-set=utf8mb4`，然后重新执行 `seed-cli-locales.sql` 覆盖修复。
