package db

import (
	"context"
	"fmt"
	"time"
)

type Upload struct {
	Filename      string
	PublicKey     string
	PublicURL     string
	Bucket        string
	ObjectKey     string
	Location      string
	ETag          string
	ContentLength int64
	ContentType   string
	CreatedAt     time.Time
}

func SaveUpload(ctx context.Context, upload Upload) error {
	db, err := Conn()
	if err != nil {
		return err
	}

	if upload.CreatedAt.IsZero() {
		upload.CreatedAt = time.Now().UTC()
	}

	_, err = db.ExecContext(
		ctx,
		`INSERT INTO uploads (
			filename,
			public_key,
			public_url,
			bucket,
			object_key,
			location,
			etag,
			content_length,
			content_type,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		upload.Filename,
		upload.PublicKey,
		upload.PublicURL,
		upload.Bucket,
		upload.ObjectKey,
		upload.Location,
		upload.ETag,
		upload.ContentLength,
		upload.ContentType,
		upload.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save upload metadata: %w", err)
	}

	return nil
}
