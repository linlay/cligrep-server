# cligrep-server

Go `1.26` backend for CLI Grep.

## Deployment Model

`cligrep-server` is deployed as a native Go application on a Linux host.
The backend itself is not containerized and does not require `docker-compose.yml`.

Docker is still a runtime prerequisite because the application shells out to host Docker
to execute BusyBox and Python sandboxes.

Docker network creation is not part of the backend deployment path for this phase.

## Host Prerequisites

- Linux host with Docker Engine and Docker CLI available in `PATH`
- Go `1.26` toolchain if you build from source
- Access to pull sandbox images or pre-load them on the host

Pull the required sandbox images before running the service:

```bash
docker pull busybox:1.36.1
docker pull python:3.12-slim
```

## Build

Build the Linux binary from the project root:

```bash
mkdir -p build
go build -o build/cligrep-server ./cmd/server
```

If you need a Linux binary from another OS, use a cross-build such as:

```bash
GOOS=linux GOARCH=amd64 go build -o build/cligrep-server ./cmd/server
```

## Recommended Runtime Layout

Example target layout on a Linux server:

```text
/opt/cligrep-server/
  bin/cligrep-server
  data/cligrep.db
  cligrep-server.env
```

Create the directories:

```bash
sudo mkdir -p /opt/cligrep-server/bin
sudo mkdir -p /opt/cligrep-server/data
sudo chown -R "$USER":"$USER" /opt/cligrep-server
cp build/cligrep-server /opt/cligrep-server/bin/cligrep-server
```

If you want to deploy with the existing SQLite data immediately, copy the current database file too:

```bash
cp data/cligrep.db /opt/cligrep-server/data/cligrep.db
```

This lets the server start against the existing seeded data and previously saved comments,
favorites, assets, and execution logs.

## Environment Variables

The service supports these runtime environment variables:

- `CLIGREP_HTTP_ADDR`
- `CLIGREP_DB_PATH`
- `CLIGREP_BUSYBOX_IMAGE`
- `CLIGREP_PYTHON_IMAGE`
- `CLIGREP_CONTAINER_CPUS`
- `CLIGREP_CONTAINER_MEMORY`
- `CLIGREP_COMMAND_TIMEOUT_MS`
- `CLIGREP_CORS_ORIGIN`

Recommended production-style env file at `/opt/cligrep-server/cligrep-server.env`:

```bash
CLIGREP_HTTP_ADDR=:8080
CLIGREP_DB_PATH=/opt/cligrep-server/data/cligrep.db
CLIGREP_BUSYBOX_IMAGE=busybox:1.36.1
CLIGREP_PYTHON_IMAGE=python:3.12-slim
CLIGREP_CONTAINER_CPUS=0.50
CLIGREP_CONTAINER_MEMORY=128m
CLIGREP_COMMAND_TIMEOUT_MS=4000
CLIGREP_CORS_ORIGIN=*
```

## Run Manually

Start the service with the env file loaded:

```bash
set -a
. /opt/cligrep-server/cligrep-server.env
set +a
/opt/cligrep-server/bin/cligrep-server
```

The SQLite database will be created at `CLIGREP_DB_PATH` if it does not already exist.
If you copied an existing `cligrep.db`, the service will reuse it directly.

## Export SQLite To SQL

Create a SQL dump from the current SQLite database:

```bash
mkdir -p backup && sqlite3 data/cligrep.db ".output backup/cligrep-$(date +%F-%H%M%S).sql" ".dump"
```

If you prefer a fixed output filename:

```bash
mkdir -p backup
sqlite3 data/cligrep.db ".dump" > backup/cligrep.sql
```

If the service is actively writing, export during a quiet window or stop the service briefly first.

## Run With systemd

Recommended unit file: `/etc/systemd/system/cligrep-server.service`

```ini
[Unit]
Description=cligrep-server
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
WorkingDirectory=/opt/cligrep-server
EnvironmentFile=/opt/cligrep-server/cligrep-server.env
ExecStart=/opt/cligrep-server/bin/cligrep-server
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now cligrep-server
sudo systemctl status cligrep-server
```

## Verification

Health check:

```bash
curl http://127.0.0.1:8080/healthz
```

Expected response includes:

```json
{"status":"ok"}
```

Example sandbox verification:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/exec \
  -H 'Content-Type: application/json' \
  -d '{"cliSlug":"sed","line":"sed --help"}'
```

Example Python sandbox verification:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/builtin/exec \
  -H 'Content-Type: application/json' \
  -d '{"line":"create python \"echo args\""}'
```

## Notes

- The backend is native Go, but sandbox execution still depends on host Docker.
- No backend container, `docker-compose.yml`, or `cligrep-network` setup is required here.
- The frontend can point to this backend by setting `VITE_API_BASE` to the host URL of this service.
