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
	})

	return router
}
