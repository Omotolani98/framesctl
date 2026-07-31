// Package router routing
package router

import (
	"encoding/json"
	"net/http"

	"github.com/Omotolani98/framesctl/internals/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(videoHandler *handler.VideoHandler) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Get("/healthz", func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)

		_ = json.NewEncoder(writer).Encode(
			map[string]string{
				"status": "healthy",
			},
		)
	})

	router.Route("/api/v1/framesrvr", func(router chi.Router) {
		router.Post("/videos", videoHandler.Upload)
		router.Post("/uploads", videoHandler.InitiateMultipartUpload)
		router.Post("/uploads/parts", videoHandler.SignUploadPart)
		router.Post("/uploads/complete", videoHandler.CompleteMultipartUpload)
		router.Post("/uploads/abort", videoHandler.AbortMultipartUpload)
	})

	router.Route("/api/v1", func(router chi.Router) {
		router.Post("/videos/{videoID}/shares", videoHandler.CreateShare)
		router.Get("/public/shares/{token}", videoHandler.PublicPlayback)
		router.Get("/public/shares/{token}/hls/*", videoHandler.PublicHLSArtifact)
	})

	return router
}
