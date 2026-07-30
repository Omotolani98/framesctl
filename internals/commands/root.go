// Package commands holds all the commands and subcommands
package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/db"
	"github.com/Omotolani98/framesctl/internals/framesrvr"
	"github.com/Omotolani98/framesctl/internals/version"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

const publicURLBase = "https://framesrvr.run"

func InitRootCmd(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "framesctl",
		Short: "video urls on the cli",
		Long:  "A simple CLI tool to generate streamable urls for videos",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(
			cmd *cobra.Command,
			args []string,
		) error {
			return configureDebugLogger(cmd, cfg)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			listFlag, err := cmd.Flags().GetBool("list")
			if err != nil {
				return err
			}

			if listFlag {
				return runList(cmd, args)
			}

			fileFlag, err := cmd.Flags().GetString("file")
			if err != nil {
				return err
			}

			if fileFlag == "" {
				return errors.New(
					"flag --file is required; usage: framesctl --file <path-to-video>",
				)
			}

			return runUpload(cmd, cfg, fileFlag)
		},
	}

	rootCmd.PersistentFlags().Bool(
		"debug",
		false,
		"write debug logs to the framesctl debug file",
	)

	rootCmd.Flags().StringP(
		"file",
		"f",
		"",
		"path to the video file to upload",
	)

	rootCmd.Flags().BoolP(
		"list",
		"l",
		false,
		"list uploaded videos",
	)
	rootCmd.MarkFlagsMutuallyExclusive("file", "list")
	rootCmd.AddCommand(newListCmd())

	return rootCmd
}

func runUpload(
	cmd *cobra.Command,
	cfg *config.Config,
	fileFlag string,
) error {
	path, err := expandPath(fileFlag)
	if err != nil {
		return err
	}

	if err := validateVideoFile(path); err != nil {
		return err
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect video file: %w", err)
	}

	client, err := framesrvr.NewClient(cfg.APIBaseURL)
	if err != nil {
		return err
	}

	debugLogger.Debug(
		"uploading video",
		"path", path,
		"file_size", fileInfo.Size(),
		"api_url", client.UploadURL(),
	)

	started := time.Now()
	upload, err := uploadVideo(cmd, client, path)
	if err != nil {
		debugLogger.Error(
			"upload video failed",
			"duration", time.Since(started).Round(time.Millisecond),
			"err", err,
		)

		return err
	}

	debugLogger.Debug(
		"video uploaded",
		"object_key", upload.Key,
		"content_length", upload.ContentLength,
		"duration", time.Since(started).Round(time.Millisecond),
	)

	key, err := newURLKey()
	if err != nil {
		return err
	}

	publicURL := publicURLBase + "/" + key

	record := db.Upload{
		Filename:      filepath.Base(path),
		PublicKey:     key,
		PublicURL:     publicURL,
		Bucket:        upload.Bucket,
		ObjectKey:     upload.Key,
		Location:      upload.Location,
		ETag:          upload.ETag,
		ContentLength: upload.ContentLength,
		ContentType:   upload.ContentType,
	}

	if err := db.SaveUpload(cmd.Context(), record); err != nil {
		debugLogger.Error(
			"save upload metadata failed",
			"object_key", upload.Key,
			"err", err,
		)

		return fmt.Errorf(
			"video uploaded as %q but saving metadata failed: %w",
			upload.Key,
			err,
		)
	}

	debugLogger.Debug(
		"upload metadata saved",
		"public_url", publicURL,
	)

	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"uploaded %s (%d bytes)\n%s\n",
		upload.Key,
		upload.ContentLength,
		publicURL,
	)

	return err
}

func Execute(ctx context.Context) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	if err := db.Init(ctx, cfg.DatabasePath); err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer db.Close()
	defer func() {
		if err := closeDebugLogger(); err != nil {
			log.Printf("close debug log: %v", err)
		}
	}()

	root := InitRootCmd(cfg)
	if err := fang.Execute(
		ctx,
		root,
		fang.WithVersion(version.Version),
	); err != nil {
		os.Exit(1)
	}
}
