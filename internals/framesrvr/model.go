package framesrvr

import "fmt"

type FileMetadata struct {
	Filename string `json:"filename"`
}

type UploadResponse struct {
	Message       string `json:"message"`
	Bucket        string `json:"bucket"`
	Key           string `json:"key"`
	Location      string `json:"location"`
	ETag          string `json:"etag"`
	ContentLength int64  `json:"content_length"`
	ContentType   string `json:"content_type"`
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
