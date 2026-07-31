package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newPlayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "play <share-url-or-token>",
		Short: "Play a streamed video in the terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlay(cmd, args[0])
		},
	}
}

func runPlay(cmd *cobra.Command, target string) error {
	playerPath, err := framesPlayerPath()
	if err != nil {
		return err
	}

	process := exec.CommandContext(cmd.Context(), playerPath, target)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = cmd.OutOrStdout()
	process.Stderr = cmd.ErrOrStderr()

	if err := process.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("framesplayer exited with status %d", exitErr.ExitCode())
		}

		return fmt.Errorf("run framesplayer: %w", err)
	}

	return nil
}

func framesPlayerPath() (string, error) {
	if configured := os.Getenv("FRAMESPLAYER_PATH"); configured != "" {
		return configured, nil
	}

	path, err := exec.LookPath("framesplayer")
	if err != nil {
		return "", errors.New("framesplayer not found; install it or set FRAMESPLAYER_PATH")
	}

	return path, nil
}
