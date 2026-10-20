package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type correlationContextKey struct{}

var latencyBounds = [...]time.Duration{
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
}

type operationalMetrics struct {
	mu               sync.Mutex
	httpRequests     uint64
	latencyBuckets   [len(latencyBounds) + 1]uint64
	wsCommands       uint64
	wsLatencyBuckets [len(latencyBounds) + 1]uint64
	rejects          map[errorCode]uint64
	reconnects       uint64
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *statusWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return writer.ResponseWriter.Write(body)
}

func correlationID(request *http.Request) string {
	if value, ok := request.Context().Value(correlationContextKey{}).(string); ok && value != "" {
		return value
	}
	value := request.Header.Get("X-Correlation-Id")
	if validIdentifier(value, 128) {
		return value
	}
	_, value, _ = randomToken(12)
	return value
}

func (gateway *Gateway) operationalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCorrelationID := correlationID(request)
		request = request.WithContext(context.WithValue(request.Context(), correlationContextKey{}, requestCorrelationID))
		writer.Header().Set("X-Correlation-Id", requestCorrelationID)
		if request.URL.Path == "/api/v1/ws" {
			next.ServeHTTP(writer, request)
			return
		}
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: writer}
		next.ServeHTTP(recorder, request)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(started)
		gateway.metrics.observeRequest(duration)
		pattern := request.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		slog.Info("http request",
			"method", request.Method,
			"route", pattern,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"correlation_id", requestCorrelationID,
		)
	})
}

func (metrics *operationalMetrics) observeRequest(duration time.Duration) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.httpRequests++
	index := len(latencyBounds)
	for candidate, bound := range latencyBounds {
		if duration <= bound {
			index = candidate
			break
		}
	}
	metrics.latencyBuckets[index]++
}

func (metrics *operationalMetrics) observeReject(code errorCode) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.rejects[code]++
}

func (metrics *operationalMetrics) observeWSCommand(duration time.Duration) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.wsCommands++
	index := len(latencyBounds)
	for candidate, bound := range latencyBounds {
		if duration <= bound {
			index = candidate
			break
		}
	}
	metrics.wsLatencyBuckets[index]++
}

func (gateway *Gateway) metricsHandler(writer http.ResponseWriter, _ *http.Request) {
	service := gateway.service.OperationalSnapshot()
	gateway.metrics.mu.Lock()
	requests := gateway.metrics.httpRequests
	buckets := gateway.metrics.latencyBuckets
	wsCommands := gateway.metrics.wsCommands
	wsBuckets := gateway.metrics.wsLatencyBuckets
	rejects := make(map[errorCode]uint64, len(gateway.metrics.rejects))
	for code, value := range gateway.metrics.rejects {
		rejects[code] = value
	}
	reconnects := gateway.metrics.reconnects
	gateway.metrics.mu.Unlock()

	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintln(writer, "# TYPE response_ready gauge")
	_, _ = fmt.Fprintf(writer, "response_ready %d\n", boolMetric(service.AcceptingNewRooms))
	_, _ = fmt.Fprintf(writer, "response_sessions %d\n", service.Sessions)
	_, _ = fmt.Fprintf(writer, "response_rooms %d\n", service.Rooms)
	_, _ = fmt.Fprintf(writer, "response_active_rooms %d\n", service.ActiveRooms)
	_, _ = fmt.Fprintf(writer, "response_active_matches %d\n", service.Matches)
	_, _ = fmt.Fprintf(writer, "response_http_requests_total %d\n", requests)
	cumulative := uint64(0)
	for index, bound := range latencyBounds {
		cumulative += buckets[index]
		_, _ = fmt.Fprintf(writer, "response_http_request_duration_seconds_bucket{le=\"%.3f\"} %d\n", bound.Seconds(), cumulative)
	}
	cumulative += buckets[len(buckets)-1]
	_, _ = fmt.Fprintf(writer, "response_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", cumulative)
	_, _ = fmt.Fprintf(writer, "response_ws_commands_total %d\n", wsCommands)
	cumulative = 0
	for index, bound := range latencyBounds {
		cumulative += wsBuckets[index]
		_, _ = fmt.Fprintf(writer, "response_ws_command_duration_seconds_bucket{le=\"%.3f\"} %d\n", bound.Seconds(), cumulative)
	}
	cumulative += wsBuckets[len(wsBuckets)-1]
	_, _ = fmt.Fprintf(writer, "response_ws_command_duration_seconds_bucket{le=\"+Inf\"} %d\n", cumulative)
	_, _ = fmt.Fprintln(writer, "# TYPE response_rejects_total counter")
	codes := make([]string, 0, len(rejects))
	for code := range rejects {
		codes = append(codes, string(code))
	}
	sort.Strings(codes)
	for _, code := range codes {
		_, _ = fmt.Fprintf(writer, "response_rejects_total{code=\"%s\"} %d\n", strings.ReplaceAll(code, "\"", ""), rejects[errorCode(code)])
	}
	_, _ = fmt.Fprintf(writer, "response_reconnects_total %d\n", reconnects)
	_, _ = fmt.Fprintf(writer, "response_technical_aborts_total %d\n", service.TechnicalAborts)
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}
