# cligrep-server

## 1. 项目简介
`cligrep-server` 是 CLI Grep 的 Go 后端，负责提供 CLI 列表检索、详情查询、评论与收藏持久化、Mock 登录，以及基于 Docker 的受限命令执行能力。

该服务按“宿主机原生进程运行”部署，不使用 `docker compose` 编排自身；前端容器会通过反向代理访问本服务。

## 2. 快速开始
### 前置要求
- Go 1.26
- MySQL 8.0+
- Docker Engine 与 Docker CLI 可用
- 已创建 MySQL 用户并允许访问目标库
- 本机已预拉取运行沙箱所需镜像：

```bash
docker pull busybox:1.36.1
docker pull python:3.12-slim
```

### 本地启动
```bash
cp .env.example .env
./build.sh
./start.sh
```

服务默认监听 `http://127.0.0.1:11802`，并连接到 `13.212.113.109:3306/cligrep`。

### 停止服务
```bash
./stop.sh
```

### 测试
```bash
go test ./...
```

## 3. 配置说明
- 所有运行配置从 `.env.example` 复制到 `.env`。
- `.env` 不提交仓库，由 `start.sh` 在启动时自动加载。
- 配置项如下：

```dotenv
CLIGREP_HTTP_ADDR=:11802
CLIGREP_DB_HOST=13.212.113.109
CLIGREP_DB_PORT=3306
CLIGREP_DB_NAME=cligrep
CLIGREP_DB_USER=cligrep
CLIGREP_DB_PASSWORD=cligrep0@123
CLIGREP_BUSYBOX_IMAGE=busybox:1.36.1
CLIGREP_PYTHON_IMAGE=python:3.12-slim
CLIGREP_CONTAINER_CPUS=0.50
CLIGREP_CONTAINER_MEMORY=128m
CLIGREP_COMMAND_TIMEOUT_MS=4000
CLIGREP_CORS_ORIGIN=http://127.0.0.1:11801,http://localhost:11801,http://127.0.0.1:5173,http://localhost:5173
```

说明：
- `CLIGREP_HTTP_ADDR`：HTTP 监听地址。
- `CLIGREP_DB_HOST` / `CLIGREP_DB_PORT`：MySQL 地址与端口。
- `CLIGREP_DB_NAME`：应用使用的 MySQL database 名称。
- `CLIGREP_DB_USER` / `CLIGREP_DB_PASSWORD`：MySQL 登录凭据。
- `CLIGREP_BUSYBOX_IMAGE` / `CLIGREP_PYTHON_IMAGE`：沙箱运行镜像。
- `CLIGREP_CONTAINER_CPUS` / `CLIGREP_CONTAINER_MEMORY`：沙箱容器资源限制。
- `CLIGREP_COMMAND_TIMEOUT_MS`：单次命令执行超时。
- `CLIGREP_CORS_ORIGIN`：允许的跨域来源，支持 `*` 或逗号分隔多个 origin。

## 4. 部署
### 仓库内运行布局
执行脚本后，仓库内会形成如下运行目录：

```text
cligrep-server/
  build/cligrep-server
  scripts/mysql/init.sql
  logs/cligrep-server.log
  run/cligrep-server.pid
  .env
```

### 构建与启动
```bash
cp .env.example .env
./build.sh
./start.sh
```

### 初始化 MySQL
如需手工建库建表，可执行：

```bash
mysql -h 13.212.113.109 -P 3306 -u root -p < scripts/mysql/init.sql
```

应用启动时也会自动尝试创建 `cligrep` 数据库并初始化表结构；若当前账号没有建库权限，请先手工执行上面的 SQL。

### 前端联调
- 前端开发模式默认通过 Vite 代理访问 `http://127.0.0.1:11802`。
- 前端容器部署模式默认由 Nginx 把 `/api` 与 `/healthz` 转发到宿主机 `:11802`。
- 如需开放其他前端来源，修改 `.env` 中 `CLIGREP_CORS_ORIGIN` 后重启服务。

### 可选 systemd 部署
需要系统托管时，可把 `ExecStart` 指向编译后的二进制，并通过 `EnvironmentFile` 复用同一份 `.env` 内容。此方式是可选增强，不是本仓库默认运行路径。

## 5. 运维
### 查看日志
```bash
tail -f logs/cligrep-server.log
```

### 查看健康状态
```bash
curl http://127.0.0.1:11802/healthz
```

`/healthz` 现在除了基础服务状态外，还会返回：
- `sandboxReady`：宿主机沙箱是否可直接执行命令。
- `databaseHost` / `databasePort` / `databaseName`：当前连接的 MySQL 信息。
- `sandbox.dockerCli`：是否能在 `PATH` 中找到 Docker CLI。
- `sandbox.dockerDaemon`：是否能连通本机 Docker daemon。
- `sandbox.busyboxImage` / `sandbox.pythonImage`：所需镜像是否已在本地预拉取。
- `sandbox.issues`：未就绪时的具体问题列表。

### 查看进程状态
```bash
cat run/cligrep-server.pid
ps -p "$(cat run/cligrep-server.pid)"
```

### 常见排查
- 启动日志出现 `warning: sandbox is not ready`：说明服务已启动，但沙箱依赖仍有缺失，先看 `/healthz` 里的 `sandbox` 与 `issues` 字段。
- 启动失败且日志提示端口冲突：检查 `11802` 是否已被其他进程占用，或修改 `CLIGREP_HTTP_ADDR`。
- 页面跨域失败：确认 `CLIGREP_CORS_ORIGIN` 包含当前前端访问地址，并重新执行 `./stop.sh && ./start.sh`。
- 执行命令失败：确认 Docker Engine 正常运行，且 `busybox:1.36.1` 与 `python:3.12-slim` 已预拉取。
- MySQL 连接失败：确认 `CLIGREP_DB_HOST`、`CLIGREP_DB_PORT`、`CLIGREP_DB_NAME`、`CLIGREP_DB_USER`、`CLIGREP_DB_PASSWORD` 正确，且应用账号具备目标库权限。
- MySQL 表未创建：先执行 `scripts/mysql/init.sql`，再重启服务。

推荐排查命令：
```bash
mysql -h 13.212.113.109 -P 3306 -u cligrep -p -e "SHOW DATABASES;"
mysql -h 13.212.113.109 -P 3306 -u cligrep -p -D cligrep -e "SHOW TABLES;"
docker info
docker image inspect busybox:1.36.1
docker image inspect python:3.12-slim
docker pull busybox:1.36.1
docker pull python:3.12-slim
```
