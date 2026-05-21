// Package api provides the native HTTP API for controlling gonic without the
// Subsonic compatibility layer or browser admin UI.
package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"go.senan.xyz/gonic/db"
	"go.senan.xyz/gonic/playlist"
	"go.senan.xyz/gonic/scanner"
	"go.senan.xyz/gonic/transcode"
)

// Controller serves the native API routes.
type Controller struct {
	*http.ServeMux

	dbc           *db.DB
	scanner       *scanner.Scanner
	playlistStore *playlist.Store
	transcoder    transcode.Transcoder
	public        []Middleware
	private       []Middleware
}

// Option configures a Controller.
type Option func(*Controller)

// New creates a native API controller.
func New(dbc *db.DB, scannr *scanner.Scanner, playlistStore *playlist.Store, transcoder transcode.Transcoder, opts ...Option) *Controller {
	c := &Controller{
		ServeMux:      http.NewServeMux(),
		dbc:           dbc,
		scanner:       scannr,
		playlistStore: playlistStore,
		transcoder:    transcoder,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.handlePublic("/health", c.serveHealth)
	c.handlePrivate("/scan", c.serveScan)
	c.handlePrivate("/scan/status", c.serveScanStatus)
	c.handlePrivate("/playlists", c.servePlaylists)
	c.handlePrivate("/playlists/", c.servePlaylist)
	c.handlePublic("/", c.serveNotFound)
	c.handlePrivate("/stream/", c.serveStream)

	return c
}

func (c *Controller) handlePublic(pattern string, h http.HandlerFunc) {
	c.Handle(pattern, chain(h, c.public...))
}

func (c *Controller) handlePrivate(pattern string, h http.HandlerFunc) {
	c.Handle(pattern, chain(h, c.private...))
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("api: write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, apiError{Error: fmt.Sprintf(format, args...)})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "method %s not allowed", r.Method)
	return false
}

func encodePlaylistID(path string) string {
	return base64.URLEncoding.EncodeToString([]byte(path))
}

func decodePlaylistID(id string) string {
	path, _ := base64.URLEncoding.DecodeString(id)
	return string(path)
}
