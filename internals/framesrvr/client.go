package framesrvr

import (
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

func (client *Client) UploadVideo(
	ctx context.Context,
	path string,
) (*UploadResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open video file: %w", err)
	}
	defer file.Close()

	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)

	go func() {
		part, err := multipartWriter.CreateFormFile(
			"video",
			filepath.Base(path),
		)
		if err == nil {
			_, err = io.Copy(part, file)
		}

		pipeWriter.CloseWithError(
			errors.Join(err, multipartWriter.Close()),
		)
	}()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+uploadPath,
		pipeReader,
	)
	if err != nil {
		_ = pipeReader.Close()

		return nil, fmt.Errorf("build upload request: %w", err)
	}

	request.Header.Set(
		"Content-Type",
		multipartWriter.FormDataContentType(),
	)

	response, err := client.httpClient.Do(request)
	if err != nil {
		_ = pipeReader.Close()

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
