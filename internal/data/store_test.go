package data

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/linlay/cligrep-server/internal/config"
)

func TestMySQLDSNIncludesConfiguredDatabase(t *testing.T) {
	cfg := config.Config{
		DBHost:     "13.212.113.109",
		DBPort:     3306,
		DBName:     "cligrep",
		DBUser:     "cligrep",
		DBPassword: "cligrep0@123",
	}

	dsn := mysqlDSN(cfg, true)
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if parsed.DBName != "cligrep" {
		t.Fatalf("expected db name cligrep, got %q", parsed.DBName)
	}
	if parsed.User != "cligrep" {
		t.Fatalf("expected user cligrep, got %q", parsed.User)
	}
	if parsed.Passwd != "cligrep0@123" {
		t.Fatalf("expected password to round-trip, got %q", parsed.Passwd)
	}
	if parsed.Addr != "13.212.113.109:3306" {
		t.Fatalf("expected addr 13.212.113.109:3306, got %q", parsed.Addr)
	}
	if !parsed.ParseTime {
		t.Fatal("expected parseTime=true")
	}
}

func TestCreateDatabaseStatementQuotesIdentifier(t *testing.T) {
	statement := createDatabaseStatement("cligrep")
	expected := "CREATE DATABASE IF NOT EXISTS `cligrep` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
	if statement != expected {
		t.Fatalf("unexpected statement: %s", statement)
	}
}

func TestMySQLSchemaStatementsUseUppercaseTrailingUnderscoreColumns(t *testing.T) {
	statements := mysqlSchemaStatements()

	required := []string{
		"CREATE TABLE IF NOT EXISTS cli_registry",
		"SLUG_ VARCHAR(128)",
		"DISPLAY_NAME_ VARCHAR(255)",
		"CREATED_AT_ DATETIME(3)",
		"CREATE TABLE IF NOT EXISTS auth_user",
		"USERNAME_ VARCHAR(128)",
		"DISPLAY_NAME_ VARCHAR(255)",
		"UPDATED_AT_ DATETIME(3)",
		"AUTH_PROVIDER_ VARCHAR(32)",
		"AUTH_SUB_ VARCHAR(255)",
		"CREATE TABLE IF NOT EXISTS auth_local_credential",
		"PASSWORD_HASH_ VARCHAR(255)",
		"CREATE TABLE IF NOT EXISTS auth_session",
		"TOKEN_HASH_ CHAR(64)",
		"EXPIRES_AT_ DATETIME(3)",
		"CREATE TABLE IF NOT EXISTS auth_login_log",
		"AUTH_METHOD_ VARCHAR(32)",
		"LOGIN_RESULT_ VARCHAR(16)",
		"CREATE TABLE IF NOT EXISTS user_favorite",
		"USER_ID_ BIGINT UNSIGNED",
		"CLI_SLUG_ VARCHAR(128)",
		"CREATE TABLE IF NOT EXISTS user_comment",
		"BODY_ TEXT",
		"CREATE TABLE IF NOT EXISTS sandbox_execution_log",
		"EXIT_CODE_ INT",
		"DURATION_MS_ BIGINT",
		"CREATE TABLE IF NOT EXISTS sandbox_generated_asset",
		"CONTENT_ MEDIUMTEXT",
		"CREATE TABLE IF NOT EXISTS seed_execution_record",
		"SEED_KEY_ VARCHAR(128)",
	}

	joined := strings.Join(statements, "\n")
	for _, fragment := range required {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("expected schema to contain %q", fragment)
		}
	}
}

func TestHomepageSortOrderUsesMySQLTables(t *testing.T) {
	tests := map[string]string{
		"":          "user_favorite",
		"favorites": "user_favorite",
		"runs":      "sandbox_execution_log",
		"newest":    "c.CREATED_AT_",
	}

	for sort, want := range tests {
		got := homepageSortOrder(sort)
		if !strings.Contains(got, want) {
			t.Fatalf("sort %q expected %q in %q", sort, want, got)
		}
	}
}
