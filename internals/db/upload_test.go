package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveUpload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	if err := Init(ctx, path); err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer Close()

	upload := Upload{
		Filename:      "clip.mp4",
		PublicKey:     "abc123",
		PublicURL:     "https://framesrvr.run/abc123",
		Bucket:        "videos",
		ObjectKey:     "videos/2026/07/30/xyz.mp4",
		Location:      "https://s3.example/xyz.mp4",
		ETag:          "etag-1",
		ContentLength: 1024,
		ContentType:   "video/mp4",
	}

	if err := SaveUpload(ctx, upload); err != nil {
		t.Fatalf("save upload: %v", err)
	}

	var (
		filename      string
		publicKey     string
		publicURL     string
		contentLength int64
		contentType   string
		createdAt     string
	)

	err := MustConn().QueryRowContext(
		ctx,
		`SELECT filename, public_key, public_url, content_length,
			content_type, created_at FROM uploads WHERE public_key = ?`,
		"abc123",
	).Scan(
		&filename,
		&publicKey,
		&publicURL,
		&contentLength,
		&contentType,
		&createdAt,
	)
	if err != nil {
		t.Fatalf("query upload: %v", err)
	}

	if filename != "clip.mp4" || publicKey != "abc123" ||
		publicURL != "https://framesrvr.run/abc123" {
		t.Errorf(
			"row = (%q, %q, %q)",
			filename,
			publicKey,
			publicURL,
		)
	}

	if contentLength != 1024 || contentType != "video/mp4" {
		t.Errorf(
			"row = (%d, %q)",
			contentLength,
			contentType,
		)
	}

	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		t.Errorf("created_at %q not RFC3339: %v", createdAt, err)
	}

	upload.PublicKey = "dup"
	if err := SaveUpload(ctx, upload); err != nil {
		t.Fatalf("save second upload: %v", err)
	}

	if err := SaveUpload(ctx, upload); err == nil ||
		!strings.Contains(err.Error(), "save upload metadata") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}
