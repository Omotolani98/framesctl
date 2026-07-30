package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"charm.land/log/v2"
	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/spf13/cobra"
)

const debugLogFileName = "debug.log"

var (
	debugLogger = log.NewWithOptions(io.Discard, log.Options{
		Level:           log.DebugLevel,
		Prefix:          "framesctl",
		ReportTimestamp: true,
	})
	debugLogFile *os.File
)

func configureDebugLogger(
	cmd *cobra.Command,
	cfg *config.Config,
) error {
	enabled, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return err
	}

	debugLogger.SetOutput(io.Discard)
	if debugLogFile != nil {
		_ = debugLogFile.Close()
		debugLogFile = nil
	}

	if !enabled {
		return nil
	}

	path := filepath.Join(cfg.AppPath, debugLogFileName)

	appRoot, err := os.OpenRoot(cfg.AppPath)
	if err != nil {
		return fmt.Errorf("open application directory: %w", err)
	}
	defer appRoot.Close()

	file, err := appRoot.OpenFile(
		debugLogFileName,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open debug log: %w", err)
	}

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()

		return fmt.Errorf("secure debug log: %w", err)
	}

	if debugLogFile != nil {
		_ = debugLogFile.Close()
	}

	debugLogFile = file
	debugLogger.SetOutput(file)

	if _, err := fmt.Fprintf(
		cmd.ErrOrStderr(),
		"debug logging enabled: %s\n",
		path,
	); err != nil {
		return fmt.Errorf("report debug log path: %w", err)
	}

	debugLogger.Debug(
		"debug logging enabled",
		"command", cmd.CommandPath(),
	)

	return nil
}

func closeDebugLogger() error {
	debugLogger.SetOutput(io.Discard)

	if debugLogFile == nil {
		return nil
	}

	err := debugLogFile.Close()
	debugLogFile = nil

	return err
}
