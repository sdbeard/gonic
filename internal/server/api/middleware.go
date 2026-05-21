package api

import (
	"compress/gzip"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Authorizer validates an API request before private handlers run.
type Authorizer func(*http.Request) error

// CORSConfig controls cross-origin API access.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// WithMiddleware applies middleware to public and private API endpoints.
func WithMiddleware(middleware ...Middleware) Option {
	return func(c *Controller) {
		c.public = append(c.public, middleware...)
		c.private = append(c.private, middleware...)
	}
}

// WithPrivateMiddleware applies middleware only to private API endpoints.
func WithPrivateMiddleware(middleware ...Middleware) Option {
	return func(c *Controller) {
		c.private = append(c.private, middleware...)
	}
}

// WithAuthorizer protects private API endpoints with an authorization check.
func WithAuthorizer(authorizer Authorizer) Option {
	return WithPrivateMiddleware(Authorize(authorizer))
}

// Authorize rejects private requests that fail the provided authorization check.
func Authorize(authorizer Authorizer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authorizer == nil {
				writeError(w, http.StatusUnauthorized, "authorization is not configured")
				return
			}
			if err := authorizer(r); err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized: %v", err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BearerTokenAuthorizer authorizes requests with an Authorization bearer token.
func BearerTokenAuthorizer(token string) Authorizer {
	return func(r *http.Request) error {
		if token == "" {
			return errors.New("empty bearer token")
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			return errors.New("invalid bearer token")
		}
		return nil
	}
}

// CORS applies a restrictive cross-origin policy.
func CORS(cfg CORSConfig) Middleware {
	cfg = cfg.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed := writeCORSHeaders(w, r, cfg)
			if r.Method == http.MethodOptions {
				writePreflight(w, allowed)
				return
			}
			if !allowed && r.Header.Get("Origin") != "" {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (cfg CORSConfig) withDefaults() CORSConfig {
	if len(cfg.AllowedMethods) == 0 {
		cfg.AllowedMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}
	if len(cfg.AllowedHeaders) == 0 {
		cfg.AllowedHeaders = []string{"Accept", "Authorization", "Content-Type"}
	}
	return cfg
}

func writeCORSHeaders(w http.ResponseWriter, r *http.Request, cfg CORSConfig) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if !originAllowed(origin, cfg.AllowedOrigins) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", corsOrigin(origin, cfg.AllowedOrigins))
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
	if cfg.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if cfg.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
	}
	w.Header().Add("Vary", "Origin")
	return true
}

func writePreflight(w http.ResponseWriter, allowed bool) {
	if !allowed {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func originAllowed(origin string, allowed []string) bool {
	return slices.Contains(allowed, "*") || slices.Contains(allowed, origin)
}

func corsOrigin(origin string, allowed []string) string {
	if slices.Contains(allowed, "*") {
		return "*"
	}
	return origin
}

// Gzip compresses responses when the client advertises gzip support.
func Gzip() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || strings.HasPrefix(r.URL.Path, "/stream/") {
				next.ServeHTTP(w, r)
				return
			}
			gz := gzip.NewWriter(w)
			defer gz.Close()
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			next.ServeHTTP(gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w gzipResponseWriter) Write(body []byte) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write(body)
}

func (w gzipResponseWriter) Flush() {
	w.writer.Flush()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
