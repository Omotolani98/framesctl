package framesrvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestUploadVideoSuccess(t *testing.T) {
	var (
		gotFilename string
		gotContent  string
		gotPaths    []string
	)
	partBodies := map[int]string{}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		gotPaths = append(gotPaths, request.URL.Path)
		switch request.URL.Path {
		case initiateUploadPath:
			var payload InitiateUploadRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			gotFilename = payload.Filename
			writeJSONResponse(writer, http.StatusCreated, InitiateUploadResponse{
				Bucket:        "videos",
				Key:           "videos/2026/07/30/abc.mp4",
				UploadID:      "upload-1",
				PartSize:      4,
				ContentType:   "video/mp4",
				ContentLength: payload.ContentLength,
			})

		case signUploadPartPath:
			var payload SignUploadPartRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSONResponse(writer, http.StatusOK, SignUploadPartResponse{
				URL: server.URL + "/s3/part/" + strconv.Itoa(int(payload.PartNumber)),
			})

		case completeUploadPath:
			gotContent = partBodies[1] + partBodies[2] + partBodies[3] + partBodies[4]
			writeJSONResponse(writer, http.StatusCreated, UploadResponse{
				Message:       "video uploaded",
				Bucket:        "videos",
				Key:           "videos/2026/07/30/abc.mp4",
				Location:      "https://s3.example/abc.mp4",
				ETag:          "etag-final",
				ContentLength: int64(len(gotContent)),
				ContentType:   "video/mp4",
			})

		case "/s3/part/1", "/s3/part/2", "/s3/part/3", "/s3/part/4":
			partNumber, _ := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/s3/part/"))
			body, err := io.ReadAll(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			partBodies[partNumber] = string(body)
			writer.Header().Set("ETag", "etag-"+strconv.Itoa(partNumber))
			writer.WriteHeader(http.StatusOK)

		default:
			writer.WriteHeader(http.StatusNotFound)
		}
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

	if gotPaths[0] != initiateUploadPath {
		t.Errorf("first path = %q, want %q", gotPaths[0], initiateUploadPath)
	}

	if gotFilename != "clip.mp4" {
		t.Errorf("filename = %q, want clip.mp4", gotFilename)
	}

	if gotContent != "fake-video-bytes" {
		t.Errorf("content = %q, want fake-video-bytes", gotContent)
	}

	if upload.Key != "videos/2026/07/30/abc.mp4" {
		t.Errorf("key = %q", upload.Key)
	}
}

func TestUploadVideoReportsProgress(t *testing.T) {
	content := []byte(strings.Repeat("video", 4096))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case initiateUploadPath:
			writeJSONResponse(writer, http.StatusCreated, InitiateUploadResponse{
				Bucket:        "videos",
				Key:           "videos/2026/07/30/progress.mp4",
				UploadID:      "upload-1",
				PartSize:      int64(len(content)),
				ContentType:   "video/mp4",
				ContentLength: int64(len(content)),
			})
		case signUploadPartPath:
			writeJSONResponse(writer, http.StatusOK, SignUploadPartResponse{URL: server.URL + "/s3/part/1"})
		case "/s3/part/1":
			_, _ = io.Copy(io.Discard, request.Body)
			writer.Header().Set("ETag", "etag-1")
			writer.WriteHeader(http.StatusOK)
		case completeUploadPath:
			writeJSONResponse(writer, http.StatusCreated, UploadResponse{Key: "videos/2026/07/30/progress.mp4"})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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

	want := "https://framesrvr.example/api/v1/framesrvr/uploads"
	if got := client.UploadURL(); got != want {
		t.Errorf("UploadURL() = %q, want %q", got, want)
	}
}

func TestCreateShare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/api/v1/videos/video-1/shares" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		writeJSONResponse(writer, http.StatusCreated, ShareResponse{
			ID:      "share-1",
			VideoID: "video-1",
			URL:     "https://framesrvr.example/watch/token",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	share, err := client.CreateShare(context.Background(), "video-1", nil)
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	if share.URL != "https://framesrvr.example/watch/token" {
		t.Errorf("share url = %q", share.URL)
	}
}

func TestPublicPlayback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/api/v1/public/shares/token" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		writeJSONResponse(writer, http.StatusOK, PublicPlaybackResponse{
			Title:     "clip.mp4",
			Status:    "queued",
			MasterURL: "/api/v1/public/shares/token/hls/master.m3u8",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	playback, err := client.PublicPlayback(context.Background(), "token")
	if err != nil {
		t.Fatalf("public playback: %v", err)
	}

	if playback.Status != "queued" {
		t.Errorf("status = %q", playback.Status)
	}
}

func writeJSONResponse(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
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
