// Package postgres stores server-side video metadata in PostgreSQL.
package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Omotolani98/framesctl/internals/video"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool          *pgxpool.Pool
	publicBaseURL string
}

func Open(
	ctx context.Context,
	databaseURL string,
	publicBaseURL string,
) (*Store, error) {
	pool, err := pgxpool.New(ctx, strings.TrimSpace(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{
		pool:          pool,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
	if err := store.Migrate(ctx); err != nil {
		pool.Close()

		return nil, err
	}

	return store, nil
}

func (store *Store) Close() {
	store.pool.Close()
}

func (store *Store) Migrate(ctx context.Context) error {
	for _, statement := range migrationStatements {
		if _, err := store.pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("run postgres migration: %w", err)
		}
	}

	return nil
}

func (store *Store) SaveUploadSession(
	ctx context.Context,
	session video.UploadSession,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin upload session save: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`INSERT INTO videos (
			id, filename, status, bucket, object_key, content_length,
			content_type, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())`,
		session.VideoID,
		session.Filename,
		video.StatusUploading,
		session.Bucket,
		session.ObjectKey,
		session.ContentLength,
		session.ContentType,
	)
	if err != nil {
		return fmt.Errorf("save video metadata: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO upload_sessions (
			id, video_id, s3_upload_id, object_key, expected_content_type,
			expected_content_length, status, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'open', $7, now())`,
		session.ID,
		session.VideoID,
		session.S3UploadID,
		session.ObjectKey,
		session.ContentType,
		session.ContentLength,
		session.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("save upload session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit upload session save: %w", err)
	}

	return nil
}

func (store *Store) FindUploadSession(
	ctx context.Context,
	key string,
	uploadID string,
) (video.UploadSession, error) {
	row := store.pool.QueryRow(
		ctx,
		`SELECT us.id, us.video_id, v.filename, v.bucket, us.object_key,
			us.s3_upload_id, us.expected_content_length,
			us.expected_content_type, us.created_at, us.expires_at
		FROM upload_sessions us
		JOIN videos v ON v.id = us.video_id
		WHERE us.object_key = $1 AND us.s3_upload_id = $2 AND us.status = 'open'`,
		key,
		uploadID,
	)

	var session video.UploadSession
	if err := row.Scan(
		&session.ID,
		&session.VideoID,
		&session.Filename,
		&session.Bucket,
		&session.ObjectKey,
		&session.S3UploadID,
		&session.ContentLength,
		&session.ContentType,
		&session.CreatedAt,
		&session.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return video.UploadSession{}, video.ErrNotFound
		}

		return video.UploadSession{}, fmt.Errorf("find upload session: %w", err)
	}

	return session, nil
}

func (store *Store) MarkUploadQueued(
	ctx context.Context,
	session video.UploadSession,
	etag string,
) (video.Video, error) {
	jobID, err := video.NewID()
	if err != nil {
		return video.Video{}, err
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return video.Video{}, fmt.Errorf("begin upload completion: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(
		ctx,
		`UPDATE videos
		SET status = $1,
			etag = $2,
			content_length = $3,
			content_type = $4,
			updated_at = now()
		WHERE id = $5
		RETURNING id, filename, status, bucket, object_key, etag,
			content_length, content_type, hls_master_key, poster_key,
			created_at, updated_at`,
		video.StatusQueued,
		etag,
		session.ContentLength,
		session.ContentType,
		session.VideoID,
	)

	completed, err := scanVideo(row)
	if err != nil {
		return video.Video{}, err
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE upload_sessions SET status = 'completed' WHERE id = $1`,
		session.ID,
	); err != nil {
		return video.Video{}, fmt.Errorf("close upload session: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transcode_jobs (id, video_id, state, created_at, updated_at)
		VALUES ($1, $2, 'queued', now(), now())
		ON CONFLICT (video_id) WHERE state IN ('queued', 'processing') DO NOTHING`,
		jobID,
		session.VideoID,
	); err != nil {
		return video.Video{}, fmt.Errorf("queue transcode job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return video.Video{}, fmt.Errorf("commit upload completion: %w", err)
	}

	return completed, nil
}

func (store *Store) CreateShare(
	ctx context.Context,
	videoID string,
	expiresAt *time.Time,
) (video.Share, error) {
	token, err := video.NewShareToken()
	if err != nil {
		return video.Share{}, err
	}

	shareID, err := video.NewID()
	if err != nil {
		return video.Share{}, err
	}

	share := video.Share{
		ID:        shareID,
		VideoID:   videoID,
		Token:     token,
		URL:       store.publicBaseURL + "/watch/" + token,
		ExpiresAt: expiresAt,
	}

	err = store.pool.QueryRow(
		ctx,
		`INSERT INTO shares (id, video_id, token_hash, expires_at, created_at)
		SELECT $1, id, $3, $4, now()
		FROM videos
		WHERE id = $2
		RETURNING created_at`,
		share.ID,
		share.VideoID,
		tokenHash(token),
		share.ExpiresAt,
	).Scan(&share.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return video.Share{}, video.ErrNotFound
		}

		return video.Share{}, fmt.Errorf("create share: %w", err)
	}

	return share, nil
}

func (store *Store) ResolveShare(
	ctx context.Context,
	token string,
) (video.Video, video.Share, error) {
	row := store.pool.QueryRow(
		ctx,
		`SELECT v.id, v.filename, v.status, v.bucket, v.object_key, v.etag,
			v.content_length, v.content_type, v.hls_master_key, v.poster_key,
			v.created_at, v.updated_at,
			s.id, s.video_id, s.expires_at, s.created_at
		FROM shares s
		JOIN videos v ON v.id = s.video_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL`,
		tokenHash(token),
	)

	var (
		found     video.Video
		share     video.Share
		expiresAt pgtype.Timestamptz
	)
	if err := row.Scan(
		&found.ID,
		&found.Filename,
		&found.Status,
		&found.Bucket,
		&found.ObjectKey,
		&found.ETag,
		&found.ContentLength,
		&found.ContentType,
		&found.HLSMasterKey,
		&found.PosterKey,
		&found.CreatedAt,
		&found.UpdatedAt,
		&share.ID,
		&share.VideoID,
		&expiresAt,
		&share.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return video.Video{}, video.Share{}, video.ErrNotFound
		}

		return video.Video{}, video.Share{}, fmt.Errorf("resolve share: %w", err)
	}

	share.Token = token
	share.URL = store.publicBaseURL + "/watch/" + token
	if expiresAt.Valid {
		expiry := expiresAt.Time
		share.ExpiresAt = &expiry
	}
	if share.ExpiresAt != nil && !time.Now().UTC().Before(share.ExpiresAt.UTC()) {
		return video.Video{}, video.Share{}, video.ErrExpired
	}

	return found, share, nil
}

func (store *Store) ClaimTranscodeJob(
	ctx context.Context,
	workerID string,
	lease time.Duration,
) (video.TranscodeJob, error) {
	leaseSeconds := int64(lease.Seconds())
	if leaseSeconds <= 0 {
		leaseSeconds = int64((5 * time.Minute).Seconds())
	}

	row := store.pool.QueryRow(
		ctx,
		`WITH next_job AS (
			SELECT id
			FROM transcode_jobs
			WHERE state = 'queued' AND available_at <= now()
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE transcode_jobs job
		SET state = 'processing',
			attempts = attempts + 1,
			leased_by = $1,
			lease_expires_at = now() + ($2 * interval '1 second'),
			updated_at = now()
		FROM next_job, videos v
		WHERE job.id = next_job.id AND v.id = job.video_id
		RETURNING job.id, v.id, v.filename, v.status, v.bucket, v.object_key,
			v.etag, v.content_length, v.content_type, v.hls_master_key,
			v.poster_key, v.created_at, v.updated_at`,
		workerID,
		leaseSeconds,
	)

	var job video.TranscodeJob
	if err := row.Scan(
		&job.ID,
		&job.Video.ID,
		&job.Video.Filename,
		&job.Video.Status,
		&job.Video.Bucket,
		&job.Video.ObjectKey,
		&job.Video.ETag,
		&job.Video.ContentLength,
		&job.Video.ContentType,
		&job.Video.HLSMasterKey,
		&job.Video.PosterKey,
		&job.Video.CreatedAt,
		&job.Video.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return video.TranscodeJob{}, video.ErrNoJob
		}

		return video.TranscodeJob{}, fmt.Errorf("claim transcode job: %w", err)
	}

	if _, err := store.pool.Exec(
		ctx,
		`UPDATE videos SET status = $1, updated_at = now() WHERE id = $2`,
		video.StatusProcessing,
		job.Video.ID,
	); err != nil {
		return video.TranscodeJob{}, fmt.Errorf("mark video processing: %w", err)
	}

	job.Video.Status = video.StatusProcessing

	return job, nil
}

func (store *Store) MarkTranscodeReady(
	ctx context.Context,
	jobID string,
	videoID string,
	hlsMasterKey string,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transcode ready update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(
		ctx,
		`UPDATE videos
		SET status = $1, hls_master_key = $2, updated_at = now()
		WHERE id = $3`,
		video.StatusReady,
		hlsMasterKey,
		videoID,
	); err != nil {
		return fmt.Errorf("mark video ready: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE transcode_jobs
		SET state = 'completed', updated_at = now(), leased_by = NULL, lease_expires_at = NULL
		WHERE id = $1`,
		jobID,
	); err != nil {
		return fmt.Errorf("mark transcode job complete: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transcode ready update: %w", err)
	}

	return nil
}

func (store *Store) MarkTranscodeFailed(
	ctx context.Context,
	jobID string,
	videoID string,
	message string,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transcode failure update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(
		ctx,
		`UPDATE videos SET status = $1, updated_at = now() WHERE id = $2`,
		video.StatusFailed,
		videoID,
	); err != nil {
		return fmt.Errorf("mark video failed: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE transcode_jobs
		SET state = 'failed', last_error = $2, updated_at = now(), leased_by = NULL, lease_expires_at = NULL
		WHERE id = $1`,
		jobID,
		message,
	); err != nil {
		return fmt.Errorf("mark transcode job failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transcode failure update: %w", err)
	}

	return nil
}

func scanVideo(row pgx.Row) (video.Video, error) {
	var found video.Video
	if err := row.Scan(
		&found.ID,
		&found.Filename,
		&found.Status,
		&found.Bucket,
		&found.ObjectKey,
		&found.ETag,
		&found.ContentLength,
		&found.ContentType,
		&found.HLSMasterKey,
		&found.PosterKey,
		&found.CreatedAt,
		&found.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return video.Video{}, video.ErrNotFound
		}

		return video.Video{}, fmt.Errorf("read video metadata: %w", err)
	}

	return found, nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))

	return sum[:]
}

var migrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS videos (
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('uploading', 'queued', 'processing', 'ready', 'failed')),
		bucket TEXT NOT NULL,
		object_key TEXT NOT NULL,
		etag TEXT NOT NULL DEFAULT '',
		content_length BIGINT NOT NULL,
		content_type TEXT NOT NULL,
		hls_master_key TEXT NOT NULL DEFAULT '',
		poster_key TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS upload_sessions (
		id TEXT PRIMARY KEY,
		video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
		s3_upload_id TEXT NOT NULL,
		object_key TEXT NOT NULL,
		expected_content_type TEXT NOT NULL,
		expected_content_length BIGINT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('open', 'completed', 'aborted')),
		expires_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		UNIQUE (object_key, s3_upload_id)
	)`,
	`CREATE TABLE IF NOT EXISTS transcode_jobs (
		id TEXT PRIMARY KEY,
		video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
		state TEXT NOT NULL CHECK (state IN ('queued', 'processing', 'completed', 'failed')),
		attempts INTEGER NOT NULL DEFAULT 0,
		available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		leased_by TEXT,
		lease_expires_at TIMESTAMPTZ,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS transcode_jobs_active_video_idx
		ON transcode_jobs(video_id) WHERE state IN ('queued', 'processing')`,
	`CREATE TABLE IF NOT EXISTS shares (
		id TEXT PRIMARY KEY,
		video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
		token_hash BYTEA NOT NULL UNIQUE,
		expires_at TIMESTAMPTZ,
		revoked_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL
	)`,
}
