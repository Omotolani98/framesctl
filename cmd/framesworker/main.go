// Package main runs the background HLS transcoding worker.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	pgstore "github.com/Omotolani98/framesctl/internals/postgres"
	"github.com/Omotolani98/framesctl/internals/storage"
	"github.com/Omotolani98/framesctl/internals/transcode"
	"github.com/Omotolani98/framesctl/internals/video"
)

const idlePollInterval = 5 * time.Second

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

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	metadataStore, err := pgstore.Open(ctx, cfg.DatabaseURL, cfg.PublicBaseURL)
	if err != nil {
		log.Fatalf("initialize postgres metadata store: %v", err)
	}
	defer metadataStore.Close()

	videoStore, err := storage.NewVideoStore(ctx, cfg)
	if err != nil {
		log.Fatalf("initialize S3 storage: %v", err)
	}

	worker := transcode.Worker{
		Jobs:        metadataStore,
		Objects:     videoStore,
		Runner:      transcode.ExecRunner{},
		WorkerID:    cfg.WorkerID,
		FFmpegPath:  cfg.FFmpegPath,
		FFprobePath: cfg.FFprobePath,
		TempDir:     cfg.WorkerTempDir,
	}

	for {
		processed, err := worker.ProcessOne(ctx)
		if err != nil {
			if errors.Is(err, video.ErrNoJob) {
				processed = false
			} else {
				log.Printf("process transcode job: %v", err)
			}
		}

		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			log.Println("framesworker stopped")
			return
		case <-time.After(idlePollInterval):
		}
	}
}
