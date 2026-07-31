// Package framesrvr handles server for uploading content to a storage bucket
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/handler"
	pgstore "github.com/Omotolani98/framesctl/internals/postgres"
	"github.com/Omotolani98/framesctl/internals/router"
	"github.com/Omotolani98/framesctl/internals/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	videoStore, err := storage.NewVideoStore(ctx, cfg)
	if err != nil {
		log.Fatalf("initialize S3 storage: %v", err)
	}

	videoHandler := handler.NewVideoHandler(videoStore, cfg.MaxUploadBytes)
	if cfg.DatabaseURL != "" {
		metadataStore, err := pgstore.Open(ctx, cfg.DatabaseURL, cfg.PublicBaseURL)
		if err != nil {
			log.Fatalf("initialize postgres metadata store: %v", err)
		}
		defer metadataStore.Close()

		videoHandler = handler.NewVideoHandlerWithMetadata(
			videoStore,
			metadataStore,
			cfg.MaxUploadBytes,
		)
	} else {
		log.Println("DATABASE_URL is empty; server-managed shares and playback metadata are disabled")
	}

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router.New(videoHandler),

		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)

	go func() {
		fmt.Println("framesrvr serves.....")
		log.Printf("server listening on %s", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down server")

	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve HTTP: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		_ = server.Close()
	}
}
