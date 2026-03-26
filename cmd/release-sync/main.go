package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/data"
	"github.com/linlay/cligrep-server/internal/releasesync"
	"github.com/linlay/cligrep-server/internal/sandbox"
	"github.com/linlay/cligrep-server/internal/seed"
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

	if err := store.SeedCLIs(ctx, seed.ExtractSeededCLIs(ctx, sandbox.NewRunner(cfg))); err != nil {
		log.Fatalf("seed cli catalog: %v", err)
	}

	syncer := releasesync.New(cfg.ReleasesRoot, cfg.ReleasesBaseURL, store)
	results, err := syncer.Sync(ctx, os.Args[1:])
	if err != nil {
		log.Fatalf("sync releases: %v", err)
	}

	for _, result := range results {
		log.Printf("synced %s releases=%d assets=%d current=%s", result.Slug, result.ReleaseCount, result.AssetCount, result.CurrentVersion)
	}
}
