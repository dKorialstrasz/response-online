package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestBuildInfoDoesNotClaimVerifiedRules(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/build", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	var info buildinfo.Info
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if info.RulesVersion != "unverified" {
		t.Fatalf("rulesVersion = %q, want unverified", info.RulesVersion)
	}
	if info.ContentVersion != "base-cards-v1" {
		t.Fatalf("contentVersion = %q", info.ContentVersion)
	}
}
