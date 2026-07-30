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

func ListUploads(ctx context.Context) ([]Upload, error) {
	db, err := Conn()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT filename, public_key, public_url, bucket, object_key,
			location, etag, content_length, content_type, created_at
		FROM uploads
		ORDER BY datetime(created_at) DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list uploads: %w", err)
	}
	defer rows.Close()

	uploads := make([]Upload, 0)

	for rows.Next() {
		var (
			upload    Upload
			createdAt string
		)

		if err := rows.Scan(
			&upload.Filename,
			&upload.PublicKey,
			&upload.PublicURL,
			&upload.Bucket,
			&upload.ObjectKey,
			&upload.Location,
			&upload.ETag,
			&upload.ContentLength,
			&upload.ContentType,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("read upload row: %w", err)
		}

		upload.CreatedAt, err = time.Parse(
			time.RFC3339Nano,
			createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("decode upload timestamp: %w", err)
		}

		uploads = append(uploads, upload)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read uploads: %w", err)
	}

	return uploads, nil
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
