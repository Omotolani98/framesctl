package framesrvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUploadVideoSuccess(t *testing.T) {
	var (
		gotFilename      string
		gotContent       string
		gotContentLength int64
		gotPath          string
	)

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		gotPath = request.URL.Path
		gotContentLength = request.ContentLength

		mediaType, _, err := mime.ParseMediaType(
			request.Header.Get("Content-Type"),
		)
		if err != nil || mediaType != "multipart/form-data" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		reader, err := request.MultipartReader()
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		part, err := reader.NextPart()
		if err != nil || part.FormName() != "video" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		gotFilename = part.FileName()

		content, err := io.ReadAll(part)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		gotContent = string(content)

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)

		_ = json.NewEncoder(writer).Encode(UploadResponse{
			Message:       "video uploaded",
			Key:           "videos/2026/07/30/abc.mp4",
			ContentLength: int64(len(content)),
			ContentType:   "video/mp4",
		})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("fake-video-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	upload, err := client.UploadVideo(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if gotPath != uploadPath {
		t.Errorf("path = %q, want %q", gotPath, uploadPath)
	}

	if gotFilename != "clip.mp4" {
		t.Errorf("filename = %q, want clip.mp4", gotFilename)
	}

	if gotContent != "fake-video-bytes" {
		t.Errorf("content = %q, want fake-video-bytes", gotContent)
	}

	if gotContentLength <= int64(len(gotContent)) {
		t.Errorf(
			"content length = %d, want multipart length greater than file length %d",
			gotContentLength,
			len(gotContent),
		)
	}

	if upload.Key != "videos/2026/07/30/abc.mp4" {
		t.Errorf("key = %q", upload.Key)
	}
}

func TestUploadVideoReportsProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		file, _, err := request.FormFile("video")
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()

		if _, err := io.Copy(io.Discard, file); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)

		_ = json.NewEncoder(writer).Encode(UploadResponse{
			Key: "videos/2026/07/30/progress.mp4",
		})
	}))
	defer server.Close()

	content := []byte(strings.Repeat("video", 4096))
	path := filepath.Join(t.TempDir(), "progress.mp4")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var (
		mu       sync.Mutex
		progress [][2]int64
	)

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.UploadVideo(
		context.Background(),
		path,
		func(read, total int64) {
			mu.Lock()
			progress = append(progress, [2]int64{read, total})
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(progress) == 0 {
		t.Fatal("expected progress updates")
	}

	last := progress[len(progress)-1]
	if last[0] != int64(len(content)) ||
		last[1] != int64(len(content)) {
		t.Errorf(
			"last progress = (%d, %d), want (%d, %d)",
			last[0],
			last[1],
			len(content),
			len(content),
		)
	}
}

func TestMultipartUploadContentLength(t *testing.T) {
	const boundary = "framesctl-test-boundary"
	content := []byte("fake-video-bytes")

	got, err := multipartUploadContentLength(
		boundary,
		"clip.mp4",
		int64(len(content)),
	)
	if err != nil {
		t.Fatalf("multipartUploadContentLength: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.SetBoundary(boundary); err != nil {
		t.Fatal(err)
	}

	part, err := writer.CreateFormFile("video", "clip.mp4")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if got != int64(body.Len()) {
		t.Errorf("length = %d, want %d", got, body.Len())
	}
}

func TestUploadVideoAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnsupportedMediaType)

		_ = json.NewEncoder(writer).Encode(
			map[string]string{
				"error": "unsupported video extension",
			},
		)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.UploadVideo(context.Background(), path, nil)

	var apiError *Error
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want *framesrvr.Error", err)
	}

	if apiError.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d", apiError.StatusCode)
	}

	if apiError.Message != "unsupported video extension" {
		t.Errorf("message = %q", apiError.Message)
	}
}

func TestUploadVideoMissingFile(t *testing.T) {
	client, err := NewClient("http://localhost:8080")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.UploadVideo(
		context.Background(),
		filepath.Join(t.TempDir(), "missing.mp4"),
		nil,
	)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewClientInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"", "not-a-url", "localhost:8080"} {
		if _, err := NewClient(baseURL); err == nil {
			t.Errorf("NewClient(%q): expected error", baseURL)
		}
	}
}

func TestClientUploadURL(t *testing.T) {
	client, err := NewClient("https://framesrvr.example/")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	want := "https://framesrvr.example/api/v1/framesrvr/videos"
	if got := client.UploadURL(); got != want {
		t.Errorf("UploadURL() = %q, want %q", got, want)
	}
}

func TestLookupVideoType(t *testing.T) {
	videoType, ok := LookupVideoType(".MP4")
	if !ok {
		t.Fatal("expected .MP4 to be allowed")
	}

	if videoType.ContentType != "video/mp4" {
		t.Errorf("content type = %q", videoType.ContentType)
	}

	if _, ok := LookupVideoType(".txt"); ok {
		t.Error("expected .txt to be rejected")
	}
}

func TestAllowedExtensionsText(t *testing.T) {
	text := AllowedExtensionsText()

	for _, extension := range []string{
		".avi", ".m4v", ".mkv", ".mov", ".mp4", ".webm",
	} {
		if !strings.Contains(text, extension) {
			t.Errorf("allowed list %q missing %q", text, extension)
		}
	}
}
