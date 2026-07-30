package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListUploads(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	if err := Init(ctx, path); err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer Close()

	older := Upload{
		Filename:      "older.mp4",
		PublicKey:     "older-key",
		PublicURL:     "https://framesrvr.run/older-key",
		Bucket:        "videos",
		ObjectKey:     "videos/2026/07/29/older.mp4",
		Location:      "https://s3.example/older.mp4",
		ETag:          "etag-older",
		ContentLength: 1024,
		ContentType:   "video/mp4",
		CreatedAt: time.Date(
			2026, 7, 29, 10, 0, 0, 0, time.UTC,
		),
	}
	newer := older
	newer.Filename = "newer.webm"
	newer.PublicKey = "newer-key"
	newer.PublicURL = "https://framesrvr.run/newer-key"
	newer.ObjectKey = "videos/2026/07/30/newer.webm"
	newer.ContentType = "video/webm"
	newer.CreatedAt = older.CreatedAt.Add(24 * time.Hour)

	for _, upload := range []Upload{older, newer} {
		if err := SaveUpload(ctx, upload); err != nil {
			t.Fatalf("save upload: %v", err)
		}
	}

	uploads, err := ListUploads(ctx)
	if err != nil {
		t.Fatalf("list uploads: %v", err)
	}

	if len(uploads) != 2 {
		t.Fatalf("len(uploads) = %d, want 2", len(uploads))
	}

	if uploads[0].Filename != "newer.webm" ||
		uploads[1].Filename != "older.mp4" {
		t.Errorf(
			"order = (%q, %q), want newest first",
			uploads[0].Filename,
			uploads[1].Filename,
		)
	}

	if !uploads[0].CreatedAt.Equal(newer.CreatedAt) {
		t.Errorf(
			"created_at = %s, want %s",
			uploads[0].CreatedAt,
			newer.CreatedAt,
		)
	}
}

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
