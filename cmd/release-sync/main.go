package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/data"
	"github.com/linlay/cligrep-server/internal/releasesync"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("validate configuration: %v", err)
	}

	store, err := data.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("close store: %v", closeErr)
		}
	}()

	slugs := os.Args[1:]
	if len(slugs) == 0 {
		slugs = releasesync.SupportedSlugs()
	}
	for _, slug := range slugs {
		if _, err := store.GetCLI(ctx, slug); err != nil {
			if err == sql.ErrNoRows {
				log.Fatalf("missing CLI catalog entry for %s; import scripts/mysql/seed-clis.sql before running release-sync", slug)
			}
			log.Fatalf("load cli %s: %v", slug, err)
		}
	}

	syncer := releasesync.New(cfg.ReleasesRoot, cfg.ReleasesBaseURL, store)
	results, err := syncer.Sync(ctx, slugs)
	if err != nil {
		log.Fatalf("sync releases: %v", err)
	}

	for _, result := range results {
		log.Printf("synced %s releases=%d assets=%d current=%s", result.Slug, result.ReleaseCount, result.AssetCount, result.CurrentVersion)
	}
}
