package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Omotolani98/framesctl/internals/framesrvr"
	"github.com/Omotolani98/framesctl/internals/storage"
	"github.com/Omotolani98/framesctl/internals/video"
	"github.com/go-chi/chi/v5"
)

func TestInitiateMultipartUpload(t *testing.T) {
	store := &fakeMultipartStore{}
	handler := NewVideoHandler(store, 1<<20)

	body := bytes.NewBufferString(`{"filename":"clip.mp4","content_length":4}`)
	request := httptest.NewRequest(http.MethodPost, "/uploads", body)
	response := httptest.NewRecorder()

	handler.InitiateMultipartUpload(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}

	var payload framesrvr.InitiateUploadResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.UploadID != "upload-1" {
		t.Errorf("upload id = %q, want upload-1", payload.UploadID)
	}

	if store.contentType != "video/mp4" {
		t.Errorf("content type = %q, want video/mp4", store.contentType)
	}
}

func TestCompleteMultipartUploadRejectsUnorderedParts(t *testing.T) {
	handler := NewVideoHandler(&fakeMultipartStore{}, 1<<20)

	body := bytes.NewBufferString(`{
		"key":"videos/test.mp4",
		"upload_id":"upload-1",
		"content_length":4,
		"content_type":"video/mp4",
		"parts":[
			{"part_number":2,"etag":"etag-2"},
			{"part_number":1,"etag":"etag-1"}
		]
	}`)
	request := httptest.NewRequest(http.MethodPost, "/uploads/complete", body)
	response := httptest.NewRecorder()

	handler.CompleteMultipartUpload(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestPublicHLSArtifactStreamsMasterPlaylist(t *testing.T) {
	backend := &fakePlaybackBackend{
		video: video.Video{
			ID:           "video-1",
			Filename:     "clip.mp4",
			Status:       video.StatusReady,
			HLSMasterKey: "hls/video-1/generation/master.m3u8",
		},
		object: storage.Object{
			Body:          io.NopCloser(strings.NewReader("#EXTM3U\n")),
			ContentLength: 8,
			ContentType:   "application/octet-stream",
			ETag:          `"etag-1"`,
		},
	}
	handler := NewVideoHandlerWithMetadata(backend, backend, 1<<20)
	router := chi.NewRouter()
	router.Get("/api/v1/public/shares/{token}/hls/*", handler.PublicHLSArtifact)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/shares/share-token/hls/master.m3u8",
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	if backend.readKey != "hls/video-1/generation/master.m3u8" {
		t.Errorf("read key = %q", backend.readKey)
	}

	if got := response.Header().Get("Content-Type"); got != "application/vnd.apple.mpegurl" {
		t.Errorf("content type = %q", got)
	}

	if response.Body.String() != "#EXTM3U\n" {
		t.Errorf("body = %q", response.Body.String())
	}
}

func TestPublicHLSArtifactForwardsRange(t *testing.T) {
	backend := &fakePlaybackBackend{
		video: video.Video{
			ID:           "video-1",
			Filename:     "clip.mp4",
			Status:       video.StatusReady,
			HLSMasterKey: "hls/video-1/generation/master.m3u8",
		},
		object: storage.Object{
			Body:          io.NopCloser(strings.NewReader("data")),
			ContentLength: 4,
			ContentType:   "video/iso.segment",
			ContentRange:  "bytes 0-3/10",
		},
	}
	handler := NewVideoHandlerWithMetadata(backend, backend, 1<<20)
	router := chi.NewRouter()
	router.Get("/api/v1/public/shares/{token}/hls/*", handler.PublicHLSArtifact)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/shares/share-token/hls/720p/segment-00001.m4s",
		nil,
	)
	request.Header.Set("Range", "bytes=0-3")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusPartialContent)
	}

	if backend.readRange != "bytes=0-3" {
		t.Errorf("range = %q", backend.readRange)
	}

	if backend.readKey != "hls/video-1/generation/720p/segment-00001.m4s" {
		t.Errorf("read key = %q", backend.readKey)
	}
}

func TestPublicHLSArtifactRejectsTraversal(t *testing.T) {
	backend := &fakePlaybackBackend{
		video: video.Video{
			ID:           "video-1",
			Filename:     "clip.mp4",
			Status:       video.StatusReady,
			HLSMasterKey: "hls/video-1/generation/master.m3u8",
		},
	}
	handler := NewVideoHandlerWithMetadata(backend, backend, 1<<20)
	router := chi.NewRouter()
	router.Get("/api/v1/public/shares/{token}/hls/*", handler.PublicHLSArtifact)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/shares/share-token/hls/../secret.m3u8",
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	if backend.readKey != "" {
		t.Errorf("unexpected S3 read key = %q", backend.readKey)
	}
}

type fakeMultipartStore struct {
	contentType string
}

func (store *fakeMultipartStore) Upload(
	context.Context,
	string,
	string,
	io.Reader,
) (storage.UploadResult, error) {
	return storage.UploadResult{}, nil
}

func (store *fakeMultipartStore) CreateMultipartUpload(
	_ context.Context,
	key string,
	contentType string,
) (storage.MultipartUpload, error) {
	store.contentType = contentType

	return storage.MultipartUpload{
		Bucket:   "videos",
		Key:      key,
		UploadID: "upload-1",
	}, nil
}

func (store *fakeMultipartStore) SignUploadPart(
	context.Context,
	string,
	string,
	int32,
) (storage.SignedUploadPart, error) {
	return storage.SignedUploadPart{URL: "https://s3.example/part"}, nil
}

func (store *fakeMultipartStore) CompleteMultipartUpload(
	context.Context,
	string,
	string,
	[]framesrvr.CompleteUploadPart,
	int64,
	string,
) (storage.UploadResult, error) {
	return storage.UploadResult{
		Bucket:        "videos",
		Key:           "videos/test.mp4",
		Location:      "https://s3.example/test.mp4",
		ETag:          "etag-final",
		ContentLength: 4,
	}, nil
}

func (store *fakeMultipartStore) AbortMultipartUpload(
	context.Context,
	string,
	string,
) error {
	return nil
}

type fakePlaybackBackend struct {
	fakeMultipartStore

	video     video.Video
	object    storage.Object
	readKey   string
	readRange string
}

func (backend *fakePlaybackBackend) SaveUploadSession(context.Context, video.UploadSession) error {
	return nil
}

func (backend *fakePlaybackBackend) FindUploadSession(
	context.Context,
	string,
	string,
) (video.UploadSession, error) {
	return video.UploadSession{}, nil
}

func (backend *fakePlaybackBackend) MarkUploadQueued(
	context.Context,
	video.UploadSession,
	string,
) (video.Video, error) {
	return backend.video, nil
}

func (backend *fakePlaybackBackend) CreateShare(
	context.Context,
	string,
	*time.Time,
) (video.Share, error) {
	return video.Share{}, nil
}

func (backend *fakePlaybackBackend) ResolveShare(
	context.Context,
	string,
) (video.Video, video.Share, error) {
	return backend.video, video.Share{ID: "share-1", VideoID: backend.video.ID}, nil
}

func (backend *fakePlaybackBackend) ReadObject(
	_ context.Context,
	key string,
	rangeHeader string,
) (storage.Object, error) {
	backend.readKey = key
	backend.readRange = rangeHeader

	return backend.object, nil
}
