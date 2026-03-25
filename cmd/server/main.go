package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/linlay/cligrep-server/internal/api"
	"github.com/linlay/cligrep-server/internal/app"
	"github.com/linlay/cligrep-server/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatalf("create application: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			log.Printf("close application: %v", closeErr)
		}
	}()

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           api.NewHandler(application, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown http server: %v", err)
		}
	}()

	sandboxStatus := application.SandboxStatus(context.WithoutCancel(ctx))
	if !sandboxStatus.Ready {
		log.Printf(
			"warning: sandbox is not ready: issues=%s busyboxImage=%s pythonImage=%s",
			strings.Join(sandboxStatus.Issues, "; "),
			cfg.BusyBoxImage,
			cfg.PythonImage,
		)
	}

	log.Printf("cli-server listening on %s", cfg.HTTPAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen and serve: %v", err)
	}
}
