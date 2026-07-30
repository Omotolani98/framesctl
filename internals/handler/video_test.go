package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Omotolani98/framesctl/internals/framesrvr"
	"github.com/Omotolani98/framesctl/internals/storage"
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
