package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dKorialstrasz/response-online/backend/internal/application"
	"github.com/dKorialstrasz/response-online/backend/internal/transport/httpserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	address := os.Getenv("RESPONSE_HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	repositoryRoot := os.Getenv("RESPONSE_REPOSITORY_ROOT")
	if repositoryRoot == "" {
		repositoryRoot = ".."
	}
	pins, err := application.NewFileMatchPinProvider(repositoryRoot, "content/releases/m1-local.json")
	if err != nil {
		slog.Error("content pins unavailable", "error", err)
		os.Exit(1)
	}
	rooms, err := application.NewRoomService(application.RoomServiceConfig{Pins: pins})
	if err != nil {
		slog.Error("room service unavailable", "error", err)
		os.Exit(1)
	}
	go func() { _ = rooms.RunCleanup(ctx, time.Minute) }()

	var origins []string
	if configured := strings.TrimSpace(os.Getenv("RESPONSE_ALLOWED_ORIGINS")); configured != "" {
		origins = strings.Split(configured, ",")
	}
	secureCookies, err := strconv.ParseBool(defaultString(os.Getenv("RESPONSE_SECURE_COOKIES"), "false"))
	if err != nil {
		slog.Error("invalid RESPONSE_SECURE_COOKIES", "error", err)
		os.Exit(1)
	}
	limits, err := securityLimitsFromEnvironment()
	if err != nil {
		slog.Error("invalid security limits", "error", err)
		os.Exit(1)
	}
	handler, err := httpserver.NewHandlerWithConfig(httpserver.Config{
		Context: ctx, Service: rooms, AllowedOrigins: origins, SecureCookies: secureCookies,
		Limits: limits,
	})
	if err != nil {
		slog.Error("transport configuration unavailable", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("local server started", "address", address)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
		slog.Info("local server stopped")
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func securityLimitsFromEnvironment() (httpserver.SecurityLimits, error) {
	limits := httpserver.SecurityLimits{}
	integerNames := []struct {
		name   string
		target *int
	}{
		{name: "RESPONSE_CREATE_ROOMS_PER_SESSION", target: &limits.CreateRoomsPerSession},
		{name: "RESPONSE_CREATE_ROOMS_PER_IP", target: &limits.CreateRoomsPerIP},
		{name: "RESPONSE_JOINS_PER_SESSION", target: &limits.JoinsPerSession},
		{name: "RESPONSE_JOINS_PER_IP", target: &limits.JoinsPerIP},
		{name: "RESPONSE_WS_COMMANDS_PER_SECOND", target: &limits.WSCommandsPerSecond},
		{name: "RESPONSE_WS_COMMAND_BURST", target: &limits.WSCommandBurst},
		{name: "RESPONSE_JSON_DEPTH", target: &limits.JSONDepth},
	}
	for _, setting := range integerNames {
		value, err := optionalPositiveInt(setting.name)
		if err != nil {
			return httpserver.SecurityLimits{}, err
		}
		*setting.target = value
	}
	bodyBytes, err := optionalPositiveInt64("RESPONSE_HTTP_BODY_BYTES")
	if err != nil {
		return httpserver.SecurityLimits{}, err
	}
	frameBytes, err := optionalPositiveInt64("RESPONSE_WS_FRAME_BYTES")
	if err != nil {
		return httpserver.SecurityLimits{}, err
	}
	limits.HTTPBodyBytes = bodyBytes
	limits.WSFrameBytes = frameBytes
	return limits, nil
}

func optionalPositiveInt(name string) (int, error) {
	value, err := optionalPositiveInt64(name)
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, fmt.Errorf("%s is outside the supported integer range", name)
	}
	return int(value), nil
}

func optionalPositiveInt64(name string) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}
