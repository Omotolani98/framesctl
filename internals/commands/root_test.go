package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/db"
	"github.com/Omotolani98/framesctl/internals/framesrvr"
)

func TestRunUploadSavesMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)

		_ = json.NewEncoder(writer).Encode(framesrvr.UploadResponse{
			Message:       "video uploaded",
			Bucket:        "videos",
			Key:           "videos/2026/07/30/xyz.mp4",
			Location:      "https://s3.example/xyz.mp4",
			ETag:          "etag-1",
			ContentLength: 4,
			ContentType:   "video/mp4",
		})
	}))
	defer server.Close()

	directory := t.TempDir()

	videoPath := filepath.Join(directory, "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := db.Init(
		context.Background(),
		filepath.Join(directory, "framesctl.db"),
	); err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{APIBaseURL: server.URL}

	root := InitRootCmd(cfg)
	output := &strings.Builder{}
	root.SetOut(output)
	root.SetArgs([]string{"--file", videoPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	printed := output.String()
	if !strings.Contains(printed, "https://framesrvr.run/") {
		t.Fatalf("output %q missing public url", printed)
	}

	var (
		filename  string
		publicURL string
		objectKey string
	)

	err := db.MustConn().QueryRowContext(
		context.Background(),
		`SELECT filename, public_url, object_key FROM uploads`,
	).Scan(&filename, &publicURL, &objectKey)
	if err != nil {
		t.Fatalf("query upload: %v", err)
	}

	if filename != "clip.mp4" {
		t.Errorf("filename = %q, want clip.mp4", filename)
	}

	if objectKey != "videos/2026/07/30/xyz.mp4" {
		t.Errorf("object_key = %q", objectKey)
	}

	if !strings.Contains(printed, publicURL) {
		t.Errorf(
			"printed url not the saved one: printed %q, saved %q",
			printed,
			publicURL,
		)
	}
}
