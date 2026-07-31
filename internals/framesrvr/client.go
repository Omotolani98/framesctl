package framesrvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	uploadPath         = "/api/v1/framesrvr/videos"
	initiateUploadPath = "/api/v1/framesrvr/uploads"
	signUploadPartPath = "/api/v1/framesrvr/uploads/parts"
	completeUploadPath = "/api/v1/framesrvr/uploads/complete"
	abortUploadPath    = "/api/v1/framesrvr/uploads/abort"
	publicSharePath    = "/api/v1/public/shares/"
	maxUploadRetries   = 3
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid api.base_url %q", baseURL)
	}

	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{},
	}, nil
}

func (client *Client) UploadURL() string {
	return client.baseURL + initiateUploadPath
}

func (client *Client) CreateShare(
	ctx context.Context,
	videoID string,
	expiresAt *time.Time,
) (*ShareResponse, error) {
	if strings.TrimSpace(videoID) == "" {
		return nil, errors.New("video id is required")
	}

	request := CreateShareRequest{}
	if expiresAt != nil {
		request.ExpiresAt = expiresAt
	}

	path := "/api/v1/videos/" + url.PathEscape(videoID) + "/shares"
	var response ShareResponse
	if err := client.postJSON(ctx, path, request, http.StatusCreated, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (client *Client) PublicPlayback(
	ctx context.Context,
	token string,
) (*PublicPlaybackResponse, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("share token is required")
	}

	var response PublicPlaybackResponse
	if err := client.getJSON(
		ctx,
		publicSharePath+url.PathEscape(token),
		http.StatusOK,
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (client *Client) UploadVideo(
	ctx context.Context,
	path string,
	progress func(read, total int64),
) (*UploadResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open video file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect video file: %w", err)
	}

	initiate, err := client.initiateUpload(ctx, filepath.Base(path), fileInfo.Size())
	if err != nil {
		return nil, err
	}

	abortOnFailure := true
	defer func() {
		if abortOnFailure {
			_ = client.abortUpload(context.Background(), initiate.Key, initiate.UploadID)
		}
	}()

	parts, err := client.uploadParts(ctx, file, fileInfo.Size(), initiate, progress)
	if err != nil {
		return nil, err
	}

	upload, err := client.completeUpload(ctx, initiate, parts)
	if err != nil {
		return nil, err
	}

	abortOnFailure = false

	return upload, nil
}

func (client *Client) uploadVideoThroughAPI(
	ctx context.Context,
	path string,
	progress func(read, total int64),
) (*UploadResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open video file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect video file: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	contentLength, err := multipartUploadContentLength(
		multipartWriter.Boundary(),
		filepath.Base(path),
		fileInfo.Size(),
	)
	if err != nil {
		return nil, err
	}

	writerDone := make(chan struct{})

	go func() {
		defer close(writerDone)

		part, err := multipartWriter.CreateFormFile(
			"video",
			filepath.Base(path),
		)
		if err == nil {
			destination := io.Writer(part)
			if progress != nil {
				destination = &progressWriter{
					writer:     part,
					total:      fileInfo.Size(),
					onProgress: progress,
				}
			}

			_, err = io.Copy(destination, file)
		}

		pipeWriter.CloseWithError(
			errors.Join(err, multipartWriter.Close()),
		)
	}()

	defer func() {
		_ = pipeReader.Close()
		<-writerDone
	}()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+uploadPath,
		pipeReader,
	)
	if err != nil {
		return nil, fmt.Errorf("build upload request: %w", err)
	}

	request.Header.Set(
		"Content-Type",
		multipartWriter.FormDataContentType(),
	)
	request.ContentLength = contentLength

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call framesrvr API: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read framesrvr response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, decodeAPIError(response.StatusCode, body)
	}

	var upload UploadResponse
	if err := json.Unmarshal(body, &upload); err != nil {
		return nil, fmt.Errorf("decode framesrvr response: %w", err)
	}

	return &upload, nil
}

func (client *Client) initiateUpload(
	ctx context.Context,
	filename string,
	contentLength int64,
) (*InitiateUploadResponse, error) {
	request := InitiateUploadRequest{
		Filename:      filename,
		ContentLength: contentLength,
	}

	var response InitiateUploadResponse
	if err := client.postJSON(ctx, initiateUploadPath, request, http.StatusCreated, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (client *Client) uploadParts(
	ctx context.Context,
	file *os.File,
	fileSize int64,
	initiate *InitiateUploadResponse,
	progress func(read, total int64),
) ([]CompleteUploadPart, error) {
	if initiate.PartSize <= 0 {
		return nil, errors.New("framesrvr returned invalid part size")
	}

	partCount := int((fileSize + initiate.PartSize - 1) / initiate.PartSize)
	parts := make([]CompleteUploadPart, 0, partCount)
	var uploaded int64

	for partNumber := 1; partNumber <= partCount; partNumber++ {
		offset := int64(partNumber-1) * initiate.PartSize
		partSize := min(initiate.PartSize, fileSize-offset)

		signed, err := client.signUploadPart(ctx, initiate.Key, initiate.UploadID, int32(partNumber))
		if err != nil {
			return nil, err
		}

		etag, err := client.uploadPartWithRetries(
			ctx,
			file,
			signed.URL,
			offset,
			partSize,
			func(count int64) {
				if progress != nil {
					progress(uploaded+count, fileSize)
				}
			},
		)
		if err != nil {
			return nil, fmt.Errorf("upload part %d: %w", partNumber, err)
		}

		uploaded += partSize
		if progress != nil {
			progress(uploaded, fileSize)
		}

		parts = append(
			parts,
			CompleteUploadPart{
				PartNumber: int32(partNumber),
				ETag:       etag,
			},
		)
	}

	return parts, nil
}

func (client *Client) signUploadPart(
	ctx context.Context,
	key string,
	uploadID string,
	partNumber int32,
) (*SignUploadPartResponse, error) {
	request := SignUploadPartRequest{
		Key:        key,
		UploadID:   uploadID,
		PartNumber: partNumber,
	}

	var response SignUploadPartResponse
	if err := client.postJSON(ctx, signUploadPartPath, request, http.StatusOK, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (client *Client) uploadPartWithRetries(
	ctx context.Context,
	file *os.File,
	url string,
	offset int64,
	partSize int64,
	progress func(read int64),
) (string, error) {
	var lastErr error

	for attempt := 1; attempt <= maxUploadRetries; attempt++ {
		etag, err := client.uploadPart(ctx, file, url, offset, partSize, progress)
		if err == nil {
			return etag, nil
		}

		lastErr = err
	}

	return "", lastErr
}

func (client *Client) uploadPart(
	ctx context.Context,
	file *os.File,
	url string,
	offset int64,
	partSize int64,
	progress func(read int64),
) (string, error) {
	reader := io.Reader(io.NewSectionReader(file, offset, partSize))
	if progress != nil {
		reader = &progressReader{
			reader:     reader,
			onProgress: progress,
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, url, reader)
	if err != nil {
		return "", fmt.Errorf("build S3 part request: %w", err)
	}
	request.ContentLength = partSize

	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("upload S3 part: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read S3 part response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("S3 part upload returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	etag := response.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("S3 part upload did not return ETag")
	}

	return etag, nil
}

func (client *Client) completeUpload(
	ctx context.Context,
	initiate *InitiateUploadResponse,
	parts []CompleteUploadPart,
) (*UploadResponse, error) {
	sort.Slice(parts, func(left, right int) bool {
		return parts[left].PartNumber < parts[right].PartNumber
	})

	request := CompleteUploadRequest{
		Key:           initiate.Key,
		UploadID:      initiate.UploadID,
		ContentLength: initiate.ContentLength,
		ContentType:   initiate.ContentType,
		Parts:         parts,
	}

	var response UploadResponse
	if err := client.postJSON(ctx, completeUploadPath, request, http.StatusCreated, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (client *Client) abortUpload(
	ctx context.Context,
	key string,
	uploadID string,
) error {
	request := AbortUploadRequest{Key: key, UploadID: uploadID}
	return client.postJSON(ctx, abortUploadPath, request, http.StatusNoContent, nil)
}

func (client *Client) postJSON(
	ctx context.Context,
	path string,
	payload any,
	wantStatus int,
	destination any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode framesrvr request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+path,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build framesrvr request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.ContentLength = int64(len(body))

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call framesrvr API: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read framesrvr response: %w", err)
	}

	if response.StatusCode != wantStatus {
		return decodeAPIError(response.StatusCode, responseBody)
	}

	if destination == nil {
		return nil
	}

	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode framesrvr response: %w", err)
	}

	return nil
}

func (client *Client) getJSON(
	ctx context.Context,
	path string,
	wantStatus int,
	destination any,
) error {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		client.baseURL+path,
		nil,
	)
	if err != nil {
		return fmt.Errorf("build framesrvr request: %w", err)
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call framesrvr API: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read framesrvr response: %w", err)
	}

	if response.StatusCode != wantStatus {
		return decodeAPIError(response.StatusCode, responseBody)
	}

	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode framesrvr response: %w", err)
	}

	return nil
}

func multipartUploadContentLength(
	boundary string,
	filename string,
	fileSize int64,
) (int64, error) {
	var overhead bytes.Buffer

	writer := multipart.NewWriter(&overhead)
	if err := writer.SetBoundary(boundary); err != nil {
		return 0, fmt.Errorf("set multipart boundary: %w", err)
	}

	if _, err := writer.CreateFormFile("video", filename); err != nil {
		return 0, fmt.Errorf("calculate multipart header: %w", err)
	}

	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("calculate multipart footer: %w", err)
	}

	return int64(overhead.Len()) + fileSize, nil
}

type progressWriter struct {
	writer     io.Writer
	read       int64
	total      int64
	onProgress func(read, total int64)
}

func (writer *progressWriter) Write(
	buffer []byte,
) (int, error) {
	count, err := writer.writer.Write(buffer)
	writer.read += int64(count)
	writer.onProgress(writer.read, writer.total)

	return count, err
}

type progressReader struct {
	reader     io.Reader
	read       int64
	onProgress func(read int64)
}

func (reader *progressReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.read += int64(count)
	reader.onProgress(reader.read)

	return count, err
}

func decodeAPIError(statusCode int, body []byte) error {
	var payload struct {
		Message string `json:"error"`
	}

	if err := json.Unmarshal(body, &payload); err != nil ||
		payload.Message == "" {
		payload.Message = http.StatusText(statusCode)
	}

	return &Error{
		StatusCode: statusCode,
		Message:    payload.Message,
	}
}
