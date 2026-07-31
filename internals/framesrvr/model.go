package framesrvr

import (
	"fmt"
	"time"
)

const UploadPartSize int64 = 16 << 20 // 16 MiB

type FileMetadata struct {
	Filename string `json:"filename"`
}

type InitiateUploadRequest struct {
	Filename      string `json:"filename"`
	ContentLength int64  `json:"content_length"`
}

type InitiateUploadResponse struct {
	VideoID       string `json:"video_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Bucket        string `json:"bucket"`
	Key           string `json:"key"`
	UploadID      string `json:"upload_id"`
	PartSize      int64  `json:"part_size"`
	ContentType   string `json:"content_type"`
	ContentLength int64  `json:"content_length"`
}

type SignUploadPartRequest struct {
	Key        string `json:"key"`
	UploadID   string `json:"upload_id"`
	PartNumber int32  `json:"part_number"`
}

type SignUploadPartResponse struct {
	URL string `json:"url"`
}

type CompleteUploadPart struct {
	PartNumber int32  `json:"part_number"`
	ETag       string `json:"etag"`
}

type CompleteUploadRequest struct {
	Key           string               `json:"key"`
	UploadID      string               `json:"upload_id"`
	ContentLength int64                `json:"content_length"`
	ContentType   string               `json:"content_type"`
	Parts         []CompleteUploadPart `json:"parts"`
}

type AbortUploadRequest struct {
	Key      string `json:"key"`
	UploadID string `json:"upload_id"`
}

type UploadResponse struct {
	Message       string `json:"message"`
	VideoID       string `json:"video_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Bucket        string `json:"bucket"`
	Key           string `json:"key"`
	Location      string `json:"location"`
	ETag          string `json:"etag"`
	ContentLength int64  `json:"content_length"`
	ContentType   string `json:"content_type"`
}

type CreateShareRequest struct {
	ExpiresAt *time.Time `json:"expires_at"`
}

type ShareResponse struct {
	ID        string     `json:"id"`
	VideoID   string     `json:"video_id"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type PublicPlaybackResponse struct {
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	MasterURL string     `json:"master_url,omitempty"`
	PosterURL string     `json:"poster_url,omitempty"`
}

type Error struct {
	StatusCode int
	Message    string
}

func (err *Error) Error() string {
	return fmt.Sprintf(
		"framesrvr API returned status %d: %s",
		err.StatusCode,
		err.Message,
	)
}
