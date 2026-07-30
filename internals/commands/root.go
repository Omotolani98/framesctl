// Package commands holds all the commands and subcommands
package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
		RunE: func(cmd *cobra.Command, args []string) error {
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

	rootCmd.Flags().StringP(
		"file",
		"f",
		"",
		"path to the video file to upload",
	)

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

	client, err := framesrvr.NewClient(cfg.APIBaseURL)
	if err != nil {
		return err
	}

	upload, err := client.UploadVideo(cmd.Context(), path)
	if err != nil {
		return err
	}

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
		return fmt.Errorf(
			"video uploaded as %q but saving metadata failed: %w",
			upload.Key,
			err,
		)
	}

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

	root := InitRootCmd(cfg)
	if err := fang.Execute(
		root.Context(),
		root,
		fang.WithVersion(version.Version),
	); err != nil {
		os.Exit(1)
	}
}
