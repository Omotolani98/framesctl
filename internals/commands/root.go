// Package commands holds all the commands and subcommands
package commands

import (
	"context"
	"log"
	"os"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/db"
	"github.com/Omotolani98/framesctl/internals/version"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

func InitRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "framesctl",
		Short: "video urls on the cli",
		Long:  "A simple CLI tool to generate streamable urls for videos",
	}

	return rootCmd
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

	root := InitRootCmd()
	if err := fang.Execute(
		root.Context(),
		root,
		fang.WithVersion(version.Version),
	); err != nil {
		os.Exit(1)
	}
}
