package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/db"
)

func TestListAliases(t *testing.T) {
	directory := t.TempDir()
	initCommandTestDB(t, filepath.Join(directory, "framesctl.db"))

	newest := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	saveCommandTestUpload(
		t,
		"older-key",
		"older.mp4",
		"video/mp4",
		1024,
		newest.Add(-24*time.Hour),
	)
	saveCommandTestUpload(
		t,
		"newer-key",
		"newer.webm",
		"video/webm",
		2048,
		newest,
	)

	cfg := &config.Config{AppPath: directory}

	listOutput, _, err := executeCommand(cfg, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	lsOutput, _, err := executeCommand(cfg, "ls")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}

	shortOutput, _, err := executeCommand(cfg, "-l")
	if err != nil {
		t.Fatalf("-l: %v", err)
	}

	if listOutput != lsOutput || listOutput != shortOutput {
		t.Fatalf(
			"alias outputs differ:\nlist:\n%s\nls:\n%s\n-l:\n%s",
			listOutput,
			lsOutput,
			shortOutput,
		)
	}

	for _, expected := range []string{
		"FILENAME",
		"SIZE",
		"TYPE",
		"UPLOADED",
		"URL",
		"older.mp4",
		"newer.webm",
		"1.0 KiB",
		"2.0 KiB",
		"https://framesrvr.run/newer-key",
	} {
		if !strings.Contains(listOutput, expected) {
			t.Errorf("output missing %q:\n%s", expected, listOutput)
		}
	}

	newerIndex := strings.Index(listOutput, "newer.webm")
	olderIndex := strings.Index(listOutput, "older.mp4")
	if newerIndex < 0 || olderIndex < 0 || newerIndex > olderIndex {
		t.Errorf("uploads not newest first:\n%s", listOutput)
	}

	if strings.Contains(listOutput, "\x1b[") {
		t.Errorf("redirected output contains ANSI styles:\n%q", listOutput)
	}
}

func TestFileAndListAreMutuallyExclusive(t *testing.T) {
	directory := t.TempDir()
	initCommandTestDB(t, filepath.Join(directory, "framesctl.db"))

	_, _, err := executeCommand(
		&config.Config{AppPath: directory},
		"--file",
		"clip.mp4",
		"--list",
	)
	if err == nil {
		t.Fatal("expected --file with --list to fail")
	}
}

func TestListEmpty(t *testing.T) {
	directory := t.TempDir()
	initCommandTestDB(t, filepath.Join(directory, "framesctl.db"))

	output, _, err := executeCommand(
		&config.Config{AppPath: directory},
		"list",
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if output != "No uploads yet.\n" {
		t.Errorf("output = %q, want empty message", output)
	}
}

func TestDebugLogging(t *testing.T) {
	directory := t.TempDir()
	initCommandTestDB(t, filepath.Join(directory, "framesctl.db"))
	defer func() {
		if err := closeDebugLogger(); err != nil {
			t.Errorf("close debug log: %v", err)
		}
	}()

	output, stderr, err := executeCommand(
		&config.Config{AppPath: directory},
		"list",
		"--debug",
	)
	if err != nil {
		t.Fatalf("list --debug: %v", err)
	}

	debugPath := filepath.Join(directory, debugLogFileName)
	if !strings.Contains(stderr, debugPath) {
		t.Errorf("stderr %q missing debug path %q", stderr, debugPath)
	}

	if output != "No uploads yet.\n" {
		t.Errorf("output = %q, want empty message", output)
	}

	if err := closeDebugLogger(); err != nil {
		t.Fatalf("close debug log: %v", err)
	}

	content, err := os.ReadFile(debugPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}

	for _, expected := range []string{
		"debug logging enabled",
		"uploads listed",
		"count=0",
	} {
		if !strings.Contains(string(content), expected) {
			t.Errorf("debug log missing %q:\n%s", expected, content)
		}
	}

	info, err := os.Stat(debugPath)
	if err != nil {
		t.Fatalf("stat debug log: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf(
			"debug log mode = %o, want 600",
			info.Mode().Perm(),
		)
	}
}

func TestDebugLogRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	initCommandTestDB(t, filepath.Join(directory, "framesctl.db"))
	defer func() {
		_ = closeDebugLogger()
	}()

	target := filepath.Join(directory, "target.log")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	debugPath := filepath.Join(directory, debugLogFileName)
	if err := os.Symlink(target, debugPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, _, err := executeCommand(
		&config.Config{AppPath: directory},
		"list",
		"--debug",
	)
	if err == nil {
		t.Fatal("expected debug log symlink to fail")
	}

	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}

	if string(content) != "keep" {
		t.Errorf("target content = %q, want unchanged", content)
	}
}

func initCommandTestDB(t *testing.T, path string) {
	t.Helper()

	if err := db.Init(context.Background(), path); err != nil {
		t.Fatalf("init db: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
}

func saveCommandTestUpload(
	t *testing.T,
	key string,
	filename string,
	contentType string,
	contentLength int64,
	createdAt time.Time,
) {
	t.Helper()

	err := db.SaveUpload(context.Background(), db.Upload{
		Filename:      filename,
		PublicKey:     key,
		PublicURL:     "https://framesrvr.run/" + key,
		Bucket:        "videos",
		ObjectKey:     "videos/2026/07/30/" + filename,
		Location:      "https://s3.example/" + filename,
		ETag:          "etag-" + key,
		ContentLength: contentLength,
		ContentType:   contentType,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("save upload: %v", err)
	}
}

func executeCommand(
	cfg *config.Config,
	args ...string,
) (string, string, error) {
	root := InitRootCmd(cfg)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}

	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()

	return stdout.String(), stderr.String(), err
}
