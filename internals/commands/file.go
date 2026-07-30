package commands

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omotolani98/framesctl/internals/framesrvr"
)

func expandPath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	if path == "~" {
		return homeDirectory, nil
	}

	return filepath.Join(homeDirectory, path[2:]), nil
}

func validateVideoFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("video file does not exist: %s", path)
		}

		return fmt.Errorf("inspect video file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a video file: %s", path)
	}

	if info.Size() == 0 {
		return fmt.Errorf("video file is empty: %s", path)
	}

	extension := strings.ToLower(filepath.Ext(path))
	if _, allowed := framesrvr.LookupVideoType(extension); !allowed {
		return fmt.Errorf(
			"unsupported video extension %q; allowed: %s",
			extension,
			framesrvr.AllowedExtensionsText(),
		)
	}

	return nil
}

func newURLKey() (string, error) {
	var identifier [16]byte

	if _, err := rand.Read(identifier[:]); err != nil {
		return "", fmt.Errorf("generate url key: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(identifier[:]), nil
}
