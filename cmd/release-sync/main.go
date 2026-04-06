package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/data"
	"github.com/linlay/cligrep-server/internal/models"
	"github.com/linlay/cligrep-server/internal/releasesync"
)

type cliLookup interface {
	GetCLI(ctx context.Context, slug string) (models.CLI, error)
}

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

	slugs, err := resolveSyncSlugs(ctx, store, cfg.ReleasesRoot, os.Args[1:], log.Default())
	if err != nil {
		log.Fatal(err)
	}
	if len(slugs) == 0 {
		log.Printf("no eligible release directories found under %s", cfg.ReleasesRoot)
		return
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

func resolveSyncSlugs(ctx context.Context, lookup cliLookup, root string, requested []string, logger *log.Logger) ([]string, error) {
	if len(requested) > 0 {
		for _, slug := range requested {
			if _, err := lookup.GetCLI(ctx, slug); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, fmt.Errorf("missing CLI catalog entry for %s; create or import the CLI before running release-sync", slug)
				}
				return nil, fmt.Errorf("load cli %s: %w", slug, err)
			}
		}
		return requested, nil
	}

	discovered, err := releasesync.DiscoverSlugs(root)
	if err != nil {
		return nil, err
	}

	slugs := make([]string, 0, len(discovered))
	for _, slug := range discovered {
		if _, err := lookup.GetCLI(ctx, slug); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if logger != nil {
					logger.Printf("warning: skipping release directory %s without a published CLI catalog entry", slug)
				}
				continue
			}
			return nil, fmt.Errorf("load cli %s: %w", slug, err)
		}
		slugs = append(slugs, slug)
	}

	return slugs, nil
}
