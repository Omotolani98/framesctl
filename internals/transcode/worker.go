package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omotolani98/framesctl/internals/video"
)

const defaultLease = 15 * time.Minute

type Worker struct {
	Jobs        video.TranscodeStore
	Objects     ObjectStore
	Runner      CommandRunner
	WorkerID    string
	FFmpegPath  string
	FFprobePath string
	TempDir     string
	Lease       time.Duration
}

func (worker Worker) ProcessOne(ctx context.Context) (bool, error) {
	if worker.Runner == nil {
		worker.Runner = ExecRunner{}
	}

	lease := worker.Lease
	if lease <= 0 {
		lease = defaultLease
	}

	job, err := worker.Jobs.ClaimTranscodeJob(ctx, worker.WorkerID, lease)
	if err != nil {
		if errors.Is(err, video.ErrNoJob) {
			return false, nil
		}

		return false, err
	}

	if err := worker.processJob(ctx, job); err != nil {
		message := err.Error()
		if markErr := worker.Jobs.MarkTranscodeFailed(
			ctx,
			job.ID,
			job.Video.ID,
			message,
		); markErr != nil {
			return true, errors.Join(err, markErr)
		}

		return true, err
	}

	return true, nil
}

func (worker Worker) processJob(ctx context.Context, job video.TranscodeJob) error {
	workDir, err := os.MkdirTemp(worker.TempDir, "framesworker-*")
	if err != nil {
		return fmt.Errorf("create worker temp directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source"+sourceExtension(job.Video.ObjectKey))
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return fmt.Errorf("create source file: %w", err)
	}

	if err := worker.Objects.Download(ctx, job.Video.ObjectKey, sourceFile); err != nil {
		_ = sourceFile.Close()
		return err
	}
	if err := sourceFile.Close(); err != nil {
		return fmt.Errorf("close source file: %w", err)
	}

	source, err := Probe(ctx, worker.Runner, worker.FFprobePath, sourcePath)
	if err != nil {
		return err
	}

	renditions := SelectRenditions(source)
	outputDir := filepath.Join(workDir, "hls")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("create HLS output directory: %w", err)
	}

	for _, rendition := range renditions {
		if err := os.MkdirAll(filepath.Join(outputDir, rendition.Name), 0o700); err != nil {
			return fmt.Errorf("create rendition directory: %w", err)
		}
	}

	if err := RunFFmpeg(
		ctx,
		worker.Runner,
		worker.FFmpegPath,
		sourcePath,
		outputDir,
		renditions,
	); err != nil {
		return err
	}

	generation, err := video.NewID()
	if err != nil {
		return err
	}
	keyPrefix := "hls/" + job.Video.ID + "/" + generation
	artifacts, err := DiscoverArtifacts(outputDir, keyPrefix)
	if err != nil {
		return err
	}

	if len(artifacts) == 0 {
		return fmt.Errorf("ffmpeg produced no HLS artifacts")
	}

	for _, artifact := range artifacts {
		if err := worker.Objects.UploadFile(
			ctx,
			artifact.Key,
			artifact.ContentType,
			artifact.Path,
		); err != nil {
			return err
		}
	}

	return worker.Jobs.MarkTranscodeReady(
		ctx,
		job.ID,
		job.Video.ID,
		keyPrefix+"/master.m3u8",
	)
}

func sourceExtension(key string) string {
	extension := strings.ToLower(filepath.Ext(key))
	if extension == "" {
		return ".mp4"
	}

	return extension
}
