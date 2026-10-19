package httpserver

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionCookieName = "response_session"

type fixedWindow struct {
	started time.Time
	count   int
}

type httpRateLimiter struct {
	mu      sync.Mutex
	entries map[string]fixedWindow
	now     func() time.Time
}

func newHTTPRateLimiter() *httpRateLimiter {
	return &httpRateLimiter{entries: make(map[string]fixedWindow), now: time.Now}
}

func (limiter *httpRateLimiter) allow(key string, limit int, window time.Duration) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now().UTC()
	entry := limiter.entries[key]
	if entry.started.IsZero() || now.Sub(entry.started) >= window {
		limiter.entries[key] = fixedWindow{started: now, count: 1}
		return true
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	limiter.entries[key] = entry
	return true
}

type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	last     time.Time
	rate     float64
	capacity float64
}

func newTokenBucket(rate, capacity int) *tokenBucket {
	return &tokenBucket{tokens: float64(capacity), last: time.Now(), rate: float64(rate), capacity: float64(capacity)}
}

func (bucket *tokenBucket) allow() bool {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	now := time.Now()
	bucket.tokens += now.Sub(bucket.last).Seconds() * bucket.rate
	if bucket.tokens > bucket.capacity {
		bucket.tokens = bucket.capacity
	}
	bucket.last = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func randomToken(byteCount int) ([]byte, string, error) {
	value := make([]byte, byteCount)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, "", err
	}
	return value, base64.RawURLEncoding.EncodeToString(value), nil
}

func csrfToken(secret string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("response-online-csrf-v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeSessionCookie(sessionID, secret string) string {
	return sessionID + "." + secret
}

func decodeSessionCookie(value string) (string, string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "session_") || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (gateway *Gateway) credentials(request *http.Request) (sessionID, secret string, ok bool) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		return "", "", false
	}
	return decodeSessionCookie(cookie.Value)
}

func (gateway *Gateway) verifyCSRF(request *http.Request, secret string) bool {
	provided := request.Header.Get("X-CSRF-Token")
	expected := csrfToken(secret, gateway.csrfKey)
	return provided != "" && hmac.Equal([]byte(provided), []byte(expected))
}

func (gateway *Gateway) allowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	_, allowed := gateway.allowedOrigins[origin]
	return allowed
}

func (gateway *Gateway) originMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/api/v1/") || request.URL.Path == "/api/v1/build" {
			next.ServeHTTP(writer, request)
			return
		}
		origin := request.Header.Get("Origin")
		if !gateway.allowedOrigin(origin) {
			gateway.writeError(writer, request, http.StatusForbidden, codeForbidden, false, "error.origin.forbidden")
			return
		}
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
		writer.Header().Set("Vary", "Origin")
		if request.Method == http.MethodOptions {
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-CSRF-Token, X-Correlation-Id")
			writer.Header().Set("Access-Control-Max-Age", "600")
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (gateway *Gateway) decodeHTTPJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, gateway.limits.HTTPBodyBytes)
	content, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		content = []byte("{}")
	}
	if err := validateJSONDepth(content, gateway.limits.JSONDepth); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain exactly one JSON value")
	}
	return nil
}

func decodeStrictJSON(content []byte, destination any) error {
	return decodeStrictJSONWithLimits(content, destination, maxWSFrame, maxJSONDepth)
}

func (gateway *Gateway) decodeWSJSON(content []byte, destination any) error {
	return decodeStrictJSONWithLimits(content, destination, gateway.limits.WSFrameBytes, gateway.limits.JSONDepth)
}

func decodeStrictJSONWithLimits(content []byte, destination any, maximumBytes int64, maximumDepth int) error {
	if int64(len(content)) > maximumBytes {
		return fmt.Errorf("frame exceeds %d bytes", maximumBytes)
	}
	if err := validateJSONDepth(content, maximumDepth); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("frame must contain exactly one JSON value")
	}
	return nil
}

func validateJSONDepth(content []byte, maximum int) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			continue
		}
		switch delimiter {
		case '{', '[':
			depth++
			if depth > maximum {
				return fmt.Errorf("JSON nesting exceeds %d", maximum)
			}
		case '}', ']':
			depth--
		}
	}
}

func clientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func validIdentifier(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func validIdentifierList(values []string, maximumItems, maximumLength int) bool {
	if len(values) > maximumItems {
		return false
	}
	for _, value := range values {
		if !validIdentifier(value, maximumLength) {
			return false
		}
	}
	return true
}

func uniqueObjectRefs(values []objectRefPayload) bool {
	seen := make(map[objectRefPayload]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
