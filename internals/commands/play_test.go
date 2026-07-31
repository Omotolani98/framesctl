package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFramesPlayerPathUsesEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "framesplayer")
	t.Setenv("FRAMESPLAYER_PATH", path)

	got, err := framesPlayerPath()
	if err != nil {
		t.Fatalf("framesPlayerPath: %v", err)
	}

	if got != path {
		t.Errorf("path = %q, want %q", got, path)
	}
}

func TestFramesPlayerPathRequiresExecutable(t *testing.T) {
	t.Setenv("FRAMESPLAYER_PATH", "")
	t.Setenv("PATH", t.TempDir())

	_, err := framesPlayerPath()
	if err == nil {
		t.Fatal("expected missing framesplayer error")
	}
}

func TestRunPlayLaunchesConfiguredPlayer(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "marker")
	player := filepath.Join(directory, "framesplayer")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"" + marker + "\"\n"
	if err := os.WriteFile(player, []byte(script), 0o700); err != nil {
		t.Fatalf("write player script: %v", err)
	}
	t.Setenv("FRAMESPLAYER_PATH", player)

	cmd := newPlayCmd()
	cmd.SetArgs([]string{"share-token"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute play command: %v", err)
	}

	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if string(content) != "share-token" {
		t.Errorf("marker = %q", content)
	}
}
