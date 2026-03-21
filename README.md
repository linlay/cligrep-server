# cligrep-server

Go `1.26` backend for CLI Grep.

## Run

```bash
go mod tidy
go run ./cmd/server
```

The server stores SQLite data under `./data/cligrep.db` and uses Docker to run BusyBox and Python sandboxes.
# cligrep-server
