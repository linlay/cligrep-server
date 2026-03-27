package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/linlay/cligrep-server/internal/config"
)

func main() {
	ctx := context.Background()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("validate configuration: %v", err)
	}

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	if err != nil {
		log.Fatalf("open mysql database: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("close db: %v", closeErr)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping mysql database: %v", err)
	}

	body, err := os.ReadFile("scripts/mysql/seed-clis.sql")
	if err != nil {
		log.Fatalf("read seed-clis.sql: %v", err)
	}

	for _, statement := range parseSQLStatements(string(body)) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			log.Fatalf("execute seed statement: %v", err)
		}
	}

	log.Print("seeded cli catalog")
}

func mysqlDSN(cfg config.Config) string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=UTC&multiStatements=false",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)
}

func parseSQLStatements(raw string) []string {
	lines := strings.Split(raw, "\n")
	statements := make([]string, 0, 8)
	var builder strings.Builder

	flush := func() {
		statement := strings.TrimSpace(builder.String())
		if statement == "" {
			builder.Reset()
			return
		}
		statement = strings.TrimSuffix(statement, ";")
		statement = strings.TrimSpace(statement)
		if statement != "" {
			statements = append(statements, statement)
		}
		builder.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}
	flush()

	return statements
}
