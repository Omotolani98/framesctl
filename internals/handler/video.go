// Package handler exposes endpoints to upload video
package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omotolani98/framesctl/internals/storage"
)

const multipartOverheadAllowance int64 = 1 << 20 // 1 MiB

var ErrVideoTooLarge = errors.New("video exceeds maximum size")

type VideoUploader interface {
	Upload(
		ctx context.Context,
		key string,
		contentType string,
		body io.Reader,
	) (storage.UploadResult, error)
}

type VideoHandler struct {
	uploader       VideoUploader
	maxUploadBytes int64
}

type allowedVideo struct {
	contentType       string
	detectedMIMETypes map[string]struct{}
}

var allowedVideos = map[string]allowedVideo{
	".mp4": {
		contentType: "video/mp4",
		detectedMIMETypes: mimeSet(
			"video/mp4",
			"application/octet-stream",
		),
	},
	".mov": {
		contentType: "video/quicktime",
		detectedMIMETypes: mimeSet(
			"video/quicktime",
			"video/mp4",
			"application/octet-stream",
		),
	},
	".webm": {
		contentType: "video/webm",
		detectedMIMETypes: mimeSet(
			"video/webm",
			"application/octet-stream",
		),
	},
	".mkv": {
		contentType: "video/x-matroska",
		detectedMIMETypes: mimeSet(
			"video/x-matroska",
			"application/octet-stream",
		),
	},
	".avi": {
		contentType: "video/x-msvideo",
		detectedMIMETypes: mimeSet(
			"video/x-msvideo",
			"video/avi",
			"application/octet-stream",
		),
	},
	".m4v": {
		contentType: "video/x-m4v",
		detectedMIMETypes: mimeSet(
			"video/x-m4v",
			"video/mp4",
			"application/octet-stream",
		),
	},
}

func NewVideoHandler(
	uploader VideoUploader,
	maxUploadBytes int64,
) *VideoHandler {
	return &VideoHandler{
		uploader:       uploader,
		maxUploadBytes: maxUploadBytes,
	}
}

func (handler *VideoHandler) Upload(
	writer http.ResponseWriter,
	request *http.Request,
) {
	maxRequestBytes := handler.maxUploadBytes +
		multipartOverheadAllowance

	if request.ContentLength > maxRequestBytes {
		writeError(
			writer,
			http.StatusRequestEntityTooLarge,
			"request exceeds maximum upload size",
		)
		return
	}

	request.Body = http.MaxBytesReader(
		writer,
		request.Body,
		maxRequestBytes,
	)

	multipartReader, err := request.MultipartReader()
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"content type must be multipart/form-data",
		)
		return
	}

	for {
		part, err := multipartReader.NextPart()
		switch {
		case errors.Is(err, io.EOF):
			writeError(
				writer,
				http.StatusBadRequest,
				`multipart field "video" is required`,
			)
			return

		case err != nil:
			handler.writeMultipartError(writer, err)
			return
		}

		if part.FormName() != "video" || part.FileName() == "" {
			_ = part.Close()
			continue
		}

		handler.uploadPart(writer, request, part)
		return
	}
}

func (handler *VideoHandler) uploadPart(
	writer http.ResponseWriter,
	request *http.Request,
	part *multipart.Part,
) {
	defer part.Close()

	extension := strings.ToLower(
		filepath.Ext(part.FileName()),
	)

	videoType, allowed := allowedVideos[extension]
	if !allowed {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			"unsupported video extension; allowed: "+
				".mp4, .mov, .webm, .mkv, .avi, .m4v",
		)
		return
	}

	header := make([]byte, 512)

	headerSize, err := io.ReadFull(part, header)
	if err != nil &&
		!errors.Is(err, io.ErrUnexpectedEOF) &&
		!errors.Is(err, io.EOF) {
		writeError(
			writer,
			http.StatusBadRequest,
			"could not read uploaded video",
		)
		return
	}

	if headerSize == 0 {
		writeError(
			writer,
			http.StatusBadRequest,
			"uploaded video is empty",
		)
		return
	}

	detectedMIME := http.DetectContentType(header[:headerSize])

	if _, valid := videoType.detectedMIMETypes[detectedMIME]; !valid {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			fmt.Sprintf(
				"file content %q does not match extension %q",
				detectedMIME,
				extension,
			),
		)
		return
	}

	objectKey, err := newVideoKey(extension)
	if err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"could not generate video identifier",
		)
		return
	}

	// Reinsert the bytes consumed while detecting the MIME type.
	stream := io.MultiReader(
		bytes.NewReader(header[:headerSize]),
		part,
	)

	// Causes the S3 multipart operation to fail before completion when
	// the actual video exceeds the configured limit.
	limitedStream := &hardLimitReader{
		reader:    stream,
		remaining: handler.maxUploadBytes,
	}

	result, err := handler.uploader.Upload(
		request.Context(),
		objectKey,
		videoType.contentType,
		limitedStream,
	)
	if err != nil {
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.Is(err, ErrVideoTooLarge):
			writeError(
				writer,
				http.StatusRequestEntityTooLarge,
				"video exceeds maximum upload size",
			)

		case errors.As(err, &maxBytesError):
			writeError(
				writer,
				http.StatusRequestEntityTooLarge,
				"request exceeds maximum upload size",
			)

		case errors.Is(err, context.Canceled):
			writeError(
				writer,
				http.StatusRequestTimeout,
				"upload was cancelled",
			)

		default:
			writeError(
				writer,
				http.StatusBadGateway,
				"failed to upload video",
			)
		}

		return
	}

	writeJSON(
		writer,
		http.StatusCreated,
		map[string]any{
			"message":        "video uploaded",
			"bucket":         result.Bucket,
			"key":            result.Key,
			"location":       result.Location,
			"etag":           result.ETag,
			"content_length": result.ContentLength,
			"content_type":   videoType.contentType,
		},
	)
}

func (handler *VideoHandler) writeMultipartError(
	writer http.ResponseWriter,
	err error,
) {
	var maxBytesError *http.MaxBytesError

	if errors.As(err, &maxBytesError) {
		writeError(
			writer,
			http.StatusRequestEntityTooLarge,
			"request exceeds maximum upload size",
		)
		return
	}

	writeError(
		writer,
		http.StatusBadRequest,
		"invalid multipart request",
	)
}

type hardLimitReader struct {
	reader    io.Reader
	remaining int64
}

func (reader *hardLimitReader) Read(
	buffer []byte,
) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}

	if reader.remaining > 0 {
		if int64(len(buffer)) > reader.remaining {
			buffer = buffer[:reader.remaining]
		}

		count, err := reader.reader.Read(buffer)
		reader.remaining -= int64(count)

		return count, err
	}

	// The permitted bytes have been consumed. Read one additional byte
	// to determine whether the source is exactly at EOF or oversized.
	var extra [1]byte

	count, err := reader.reader.Read(extra[:])
	if count > 0 {
		return 0, ErrVideoTooLarge
	}

	return 0, err
}

func newVideoKey(extension string) (string, error) {
	var identifier [16]byte

	if _, err := rand.Read(identifier[:]); err != nil {
		return "", fmt.Errorf("generate random identifier: %w", err)
	}

	return fmt.Sprintf(
		"videos/%s/%s%s",
		time.Now().UTC().Format("2006/01/02"),
		hex.EncodeToString(identifier[:]),
		extension,
	), nil
}

func mimeSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))

	for _, value := range values {
		set[value] = struct{}{}
	}

	return set
}

func writeError(
	writer http.ResponseWriter,
	status int,
	message string,
) {
	writeJSON(
		writer,
		status,
		map[string]string{
			"error": message,
		},
	)
}

func writeJSON(
	writer http.ResponseWriter,
	status int,
	value any,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	_ = json.NewEncoder(writer).Encode(value)
}
