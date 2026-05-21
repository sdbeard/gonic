package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReportsOK(t *testing.T) {
	controller := New(nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	controller.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("health response status = %q, want ok", got.Status)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	controller := New(nil, nil, nil, nil, WithMiddleware(CORS(CORSConfig{AllowedOrigins: []string{"https://example.test"}})))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.test")
	rec := httptest.NewRecorder()

	controller.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.test" {
		t.Fatalf("allow origin = %q, want configured origin", got)
	}
}

func TestCORSDeniesUnknownOrigin(t *testing.T) {
	controller := New(nil, nil, nil, nil, WithMiddleware(CORS(CORSConfig{})))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.test")
	rec := httptest.NewRecorder()

	controller.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGzipCompressesJSONEndpoints(t *testing.T) {
	controller := New(nil, nil, nil, nil, WithMiddleware(Gzip()))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	controller.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", got)
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("open gzip body: %v", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	var got healthResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode gzip body: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("health response status = %q, want ok", got.Status)
	}
}

func TestAuthorizerProtectsPrivateEndpointsOnly(t *testing.T) {
	controller := New(nil, nil, nil, nil, WithAuthorizer(BearerTokenAuthorizer("secret")))
	req := httptest.NewRequest(http.MethodPost, "/scan", nil)
	rec := httptest.NewRecorder()

	controller.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("private endpoint status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rec = httptest.NewRecorder()
	controller.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public endpoint status = %d, want %d", rec.Code, http.StatusOK)
	}
}
