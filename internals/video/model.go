// Package video contains server-side video metadata models.
package video

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound = errors.New("video metadata not found")
	ErrExpired  = errors.New("share has expired or was revoked")
	ErrNoJob    = errors.New("no transcode job is available")
)

const (
	StatusUploading  = "uploading"
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"
)

type UploadSession struct {
	ID            string
	VideoID       string
	Filename      string
	Bucket        string
	ObjectKey     string
	S3UploadID    string
	ContentLength int64
	ContentType   string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type Video struct {
	ID            string
	Filename      string
	Status        string
	Bucket        string
	ObjectKey     string
	ETag          string
	ContentLength int64
	ContentType   string
	HLSMasterKey  string
	PosterKey     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Share struct {
	ID        string
	VideoID   string
	Token     string
	URL       string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

type TranscodeJob struct {
	ID    string
	Video Video
}

type Store interface {
	SaveUploadSession(ctx context.Context, session UploadSession) error
	FindUploadSession(ctx context.Context, key string, uploadID string) (UploadSession, error)
	MarkUploadQueued(ctx context.Context, session UploadSession, etag string) (Video, error)
	CreateShare(ctx context.Context, videoID string, expiresAt *time.Time) (Share, error)
	ResolveShare(ctx context.Context, token string) (Video, Share, error)
}

type TranscodeStore interface {
	ClaimTranscodeJob(ctx context.Context, workerID string, lease time.Duration) (TranscodeJob, error)
	MarkTranscodeReady(ctx context.Context, jobID string, videoID string, hlsMasterKey string) error
	MarkTranscodeFailed(ctx context.Context, jobID string, videoID string, message string) error
}

func NewID() (string, error) {
	return randomURLText(16)
}

func NewShareToken() (string, error) {
	return randomURLText(32)
}

func randomURLText(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
