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
	"strings"
)

const uploadPath = "/api/v1/framesrvr/videos"

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
	return client.baseURL + uploadPath
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
