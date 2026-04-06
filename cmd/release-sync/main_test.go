package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/linlay/cligrep-server/internal/models"
)

type fakeLookup struct {
	clis map[string]models.CLI
	errs map[string]error
}

func (f fakeLookup) GetCLI(ctx context.Context, slug string) (models.CLI, error) {
	_ = ctx
	if err, ok := f.errs[slug]; ok {
		return models.CLI{}, err
	}
	cli, ok := f.clis[slug]
	if !ok {
		return models.CLI{}, sql.ErrNoRows
	}
	return cli, nil
}

func TestResolveSyncSlugsRejectsMissingExplicitCLI(t *testing.T) {
	_, err := resolveSyncSlugs(context.Background(), fakeLookup{}, t.TempDir(), []string{"sampledb"}, log.New(ioDiscard{}, "", 0))
	if err == nil {
		t.Fatal("expected explicit slugs to require a catalog entry")
	}
	if !strings.Contains(err.Error(), "missing CLI catalog entry for sampledb") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSyncSlugsSkipsUnknownDiscoveredDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"sampledb", "orphaned", "mailtool"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	lookup := fakeLookup{
		clis: map[string]models.CLI{
			"sampledb": {Slug: "sampledb"},
			"mailtool": {Slug: "mailtool"},
		},
	}
	var logs bytes.Buffer
	slugs, err := resolveSyncSlugs(context.Background(), lookup, root, nil, log.New(&logs, "", 0))
	if err != nil {
		t.Fatalf("resolve sync slugs: %v", err)
	}

	want := []string{"mailtool", "sampledb"}
	if !reflect.DeepEqual(slugs, want) {
		t.Fatalf("expected slugs %v, got %v", want, slugs)
	}
	if !strings.Contains(logs.String(), "warning: skipping release directory orphaned") {
		t.Fatalf("expected warning log, got %q", logs.String())
	}
}

func TestResolveSyncSlugsPropagatesUnexpectedLookupErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sampledb"), 0o755); err != nil {
		t.Fatalf("mkdir sampledb: %v", err)
	}

	lookup := fakeLookup{
		errs: map[string]error{"sampledb": errors.New("db unavailable")},
	}
	_, err := resolveSyncSlugs(context.Background(), lookup, root, nil, log.New(ioDiscard{}, "", 0))
	if err == nil {
		t.Fatal("expected lookup error")
	}
	if !strings.Contains(err.Error(), "load cli sampledb: db unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
