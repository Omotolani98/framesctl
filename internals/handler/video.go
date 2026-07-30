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

	"github.com/Omotolani98/framesctl/internals/framesrvr"
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

type MultipartUploader interface {
	CreateMultipartUpload(
		ctx context.Context,
		key string,
		contentType string,
	) (storage.MultipartUpload, error)
	SignUploadPart(
		ctx context.Context,
		key string,
		uploadID string,
		partNumber int32,
	) (storage.SignedUploadPart, error)
	CompleteMultipartUpload(
		ctx context.Context,
		key string,
		uploadID string,
		parts []framesrvr.CompleteUploadPart,
		contentLength int64,
		contentType string,
	) (storage.UploadResult, error)
	AbortMultipartUpload(
		ctx context.Context,
		key string,
		uploadID string,
	) error
}

type VideoHandler struct {
	uploader       VideoUploader
	multipart      MultipartUploader
	maxUploadBytes int64
}

func NewVideoHandler(
	uploader interface {
		VideoUploader
		MultipartUploader
	},
	maxUploadBytes int64,
) *VideoHandler {
	return &VideoHandler{
		uploader:       uploader,
		multipart:      uploader,
		maxUploadBytes: maxUploadBytes,
	}
}

func (handler *VideoHandler) InitiateMultipartUpload(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var payload framesrvr.InitiateUploadRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}

	if payload.ContentLength <= 0 {
		writeError(writer, http.StatusBadRequest, "content_length must be greater than zero")
		return
	}

	if payload.ContentLength > handler.maxUploadBytes {
		writeError(writer, http.StatusRequestEntityTooLarge, "video exceeds maximum upload size")
		return
	}

	extension := strings.ToLower(filepath.Ext(payload.Filename))
	videoType, allowed := framesrvr.LookupVideoType(extension)
	if !allowed {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			"unsupported video extension; allowed: "+framesrvr.AllowedExtensionsText(),
		)
		return
	}

	objectKey, err := newVideoKey(extension)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not generate video identifier")
		return
	}

	upload, err := handler.multipart.CreateMultipartUpload(
		request.Context(),
		objectKey,
		videoType.ContentType,
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "failed to start upload")
		return
	}

	writeJSON(
		writer,
		http.StatusCreated,
		framesrvr.InitiateUploadResponse{
			Bucket:        upload.Bucket,
			Key:           upload.Key,
			UploadID:      upload.UploadID,
			PartSize:      framesrvr.UploadPartSize,
			ContentType:   videoType.ContentType,
			ContentLength: payload.ContentLength,
		},
	)
}

func (handler *VideoHandler) SignUploadPart(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var payload framesrvr.SignUploadPartRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}

	if payload.Key == "" || payload.UploadID == "" {
		writeError(writer, http.StatusBadRequest, "key and upload_id are required")
		return
	}

	if payload.PartNumber < 1 || payload.PartNumber > 10000 {
		writeError(writer, http.StatusBadRequest, "part_number must be between 1 and 10000")
		return
	}

	part, err := handler.multipart.SignUploadPart(
		request.Context(),
		payload.Key,
		payload.UploadID,
		payload.PartNumber,
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "failed to sign upload part")
		return
	}

	writeJSON(writer, http.StatusOK, framesrvr.SignUploadPartResponse{URL: part.URL})
}

func (handler *VideoHandler) CompleteMultipartUpload(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var payload framesrvr.CompleteUploadRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}

	if payload.Key == "" || payload.UploadID == "" {
		writeError(writer, http.StatusBadRequest, "key and upload_id are required")
		return
	}

	if payload.ContentLength <= 0 || payload.ContentLength > handler.maxUploadBytes {
		writeError(writer, http.StatusBadRequest, "content_length is invalid")
		return
	}

	if payload.ContentType == "" {
		writeError(writer, http.StatusBadRequest, "content_type is required")
		return
	}

	if len(payload.Parts) == 0 {
		writeError(writer, http.StatusBadRequest, "parts are required")
		return
	}

	if !validCompleteParts(payload.Parts) {
		writeError(writer, http.StatusBadRequest, "parts must be ordered and include ETags")
		return
	}

	result, err := handler.multipart.CompleteMultipartUpload(
		request.Context(),
		payload.Key,
		payload.UploadID,
		payload.Parts,
		payload.ContentLength,
		payload.ContentType,
	)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "failed to complete upload")
		return
	}

	writeJSON(
		writer,
		http.StatusCreated,
		framesrvr.UploadResponse{
			Message:       "video uploaded",
			Bucket:        result.Bucket,
			Key:           result.Key,
			Location:      result.Location,
			ETag:          result.ETag,
			ContentLength: result.ContentLength,
			ContentType:   payload.ContentType,
		},
	)
}

func (handler *VideoHandler) AbortMultipartUpload(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var payload framesrvr.AbortUploadRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}

	if payload.Key == "" || payload.UploadID == "" {
		writeError(writer, http.StatusBadRequest, "key and upload_id are required")
		return
	}

	if err := handler.multipart.AbortMultipartUpload(
		request.Context(),
		payload.Key,
		payload.UploadID,
	); err != nil {
		writeError(writer, http.StatusBadGateway, "failed to abort upload")
		return
	}

	writer.WriteHeader(http.StatusNoContent)
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

	videoType, allowed := framesrvr.LookupVideoType(extension)
	if !allowed {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			"unsupported video extension; allowed: "+
				framesrvr.AllowedExtensionsText(),
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

	if _, valid := videoType.DetectedMIMETypes[detectedMIME]; !valid {
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
		videoType.ContentType,
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
		framesrvr.UploadResponse{
			Message:       "video uploaded",
			Bucket:        result.Bucket,
			Key:           result.Key,
			Location:      result.Location,
			ETag:          result.ETag,
			ContentLength: result.ContentLength,
			ContentType:   videoType.ContentType,
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

func decodeJSON(
	writer http.ResponseWriter,
	request *http.Request,
	destination any,
) bool {
	defer request.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON request")
		return false
	}

	return true
}

func validCompleteParts(parts []framesrvr.CompleteUploadPart) bool {
	previous := int32(0)
	for _, part := range parts {
		if part.PartNumber <= previous || part.ETag == "" {
			return false
		}

		previous = part.PartNumber
	}

	return true
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
