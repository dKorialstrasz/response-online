// Package httpserver is the HTTP boundary of the local M1 service.
package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/dKorialstrasz/response-online/backend/internal/buildinfo"
)

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", jsonHandler(http.StatusOK, map[string]string{"status": "ok"}))
	mux.HandleFunc("GET /readyz", jsonHandler(http.StatusOK, map[string]string{"status": "ready"}))
	mux.HandleFunc("GET /api/v1/build", jsonHandler(http.StatusOK, buildinfo.Current()))
	return mux
}

func jsonHandler(status int, body any) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(status)
		if err := json.NewEncoder(writer).Encode(body); err != nil {
			return
		}
	}
}
