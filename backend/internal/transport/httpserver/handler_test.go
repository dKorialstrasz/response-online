package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/buildinfo"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func TestOperationalEndpointsAndLogsExposeOnlyAggregates(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	handler := NewHandler()

	request := httptest.NewRequest(http.MethodGet, "/healthz?ticket=never-log-this", nil)
	request.Header.Set("Cookie", "response_session=session_private.secret_private")
	request.Header.Set("X-Correlation-Id", "operation-18")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Correlation-Id") != "operation-18" {
		t.Fatalf("correlation header = %q", response.Header().Get("X-Correlation-Id"))
	}

	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	metrics := metricsResponse.Body.String()
	for _, name := range []string{"response_active_rooms", "response_active_matches", "response_http_request_duration_seconds_bucket", "response_ws_command_duration_seconds_bucket", "response_rejects_total", "response_reconnects_total", "response_technical_aborts_total"} {
		if !strings.Contains(metrics, name) {
			t.Fatalf("metrics omit %q: %s", name, metrics)
		}
	}
	logged := logs.String()
	if strings.Contains(logged, "never-log-this") || strings.Contains(logged, "secret_private") {
		t.Fatalf("structured log leaked query or cookie: %s", logged)
	}
	if !strings.Contains(logged, `"correlation_id":"operation-18"`) {
		t.Fatalf("structured log omitted correlation id: %s", logged)
	}
}

func TestReadinessChangesWhenServiceDrains(t *testing.T) {
	service, err := application.NewRoomService(application.RoomServiceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithConfig(Config{Service: service})
	if err != nil {
		t.Fatal(err)
	}
	service.BeginShutdown(context.Background())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d", response.Code)
	}
}

func TestBuildInfoPinsCoreRulesAndEngine(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/build", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	var info buildinfo.Info
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if info.RulesVersion != "response-rules-2026-10-17" {
		t.Fatalf("rulesVersion = %q", info.RulesVersion)
	}
	if info.ContentVersion != "base-cards-v1" {
		t.Fatalf("contentVersion = %q", info.ContentVersion)
	}
	if info.EngineVersion != "base-abilities-v2" {
		t.Fatalf("engineVersion = %q", info.EngineVersion)
	}
}
