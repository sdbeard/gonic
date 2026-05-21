package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.senan.xyz/gonic/playlist"
	"go.senan.xyz/gonic/server/ctrlsubsonic/specid"
	"go.senan.xyz/gonic/server/ctrlsubsonic/specidpaths"
)

const defaultAPIUserID = 1

type playlistRequest struct {
	Name    string   `json:"name"`
	Comment string   `json:"comment"`
	Public  bool     `json:"public"`
	UserID  int      `json:"userId"`
	Items   []string `json:"items"`
}

type playlistResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Comment   string    `json:"comment,omitempty"`
	Public    bool      `json:"public"`
	UserID    int       `json:"userId"`
	UpdatedAt time.Time `json:"updatedAt"`
	SongCount int       `json:"songCount"`
	Items     []string  `json:"items,omitempty"`
}

func (c *Controller) servePlaylists(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		c.listPlaylists(w)
		return
	}
	if r.Method == http.MethodPost {
		c.createPlaylist(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method %s not allowed", r.Method)
}

func (c *Controller) servePlaylist(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/playlists/")
	if r.Method == http.MethodGet {
		c.getPlaylist(w, id)
		return
	}
	if r.Method == http.MethodPut {
		c.updatePlaylist(w, r, id)
		return
	}
	if r.Method == http.MethodDelete {
		c.deletePlaylist(w, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method %s not allowed", r.Method)
}

func (c *Controller) listPlaylists(w http.ResponseWriter) {
	paths, err := c.playlistStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list playlists: %v", err)
		return
	}
	playlists, err := c.readPlaylists(paths, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read playlists: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, playlists)
}

func (c *Controller) readPlaylists(paths []string, includeItems bool) ([]playlistResponse, error) {
	responses := make([]playlistResponse, 0, len(paths))
	for _, path := range paths {
		stored, err := c.playlistStore.Read(path)
		if err != nil {
			return nil, err
		}
		responses = append(responses, renderPlaylist(path, stored, includeItems))
	}
	return responses, nil
}

func (c *Controller) getPlaylist(w http.ResponseWriter, id string) {
	path := decodePlaylistID(id)
	stored, err := c.playlistStore.Read(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "playlist %q not found", id)
		return
	}
	writeJSON(w, http.StatusOK, renderPlaylist(path, stored, true))
}

func (c *Controller) createPlaylist(w http.ResponseWriter, r *http.Request) {
	req, ok := decodePlaylistRequest(w, r)
	if !ok {
		return
	}
	stored, err := c.playlistFromRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "playlist request: %v", err)
		return
	}
	path := playlist.NewPath(stored.UserID, stored.Name)
	c.writePlaylist(w, path, stored)
}

func (c *Controller) updatePlaylist(w http.ResponseWriter, r *http.Request, id string) {
	req, ok := decodePlaylistRequest(w, r)
	if !ok {
		return
	}
	stored, err := c.playlistFromRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "playlist request: %v", err)
		return
	}
	c.writePlaylist(w, decodePlaylistID(id), stored)
}

func (c *Controller) deletePlaylist(w http.ResponseWriter, id string) {
	if err := c.playlistStore.Delete(decodePlaylistID(id)); err != nil {
		writeError(w, http.StatusInternalServerError, "delete playlist: %v", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) writePlaylist(w http.ResponseWriter, path string, stored *playlist.Playlist) {
	if err := c.playlistStore.Write(path, stored); err != nil {
		writeError(w, http.StatusInternalServerError, "write playlist: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, renderPlaylist(path, stored, true))
}

func decodePlaylistRequest(w http.ResponseWriter, r *http.Request) (playlistRequest, bool) {
	var req playlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode playlist request: %v", err)
		return req, false
	}
	return req, true
}

func (c *Controller) playlistFromRequest(req playlistRequest) (*playlist.Playlist, error) {
	items, err := c.resolvePlaylistItems(req.Items)
	if err != nil {
		return nil, err
	}
	userID := req.UserID
	if userID == 0 {
		userID = defaultAPIUserID
	}
	return &playlist.Playlist{UpdatedAt: time.Now(), UserID: userID, Name: req.Name, Comment: req.Comment, IsPublic: req.Public, Items: items}, nil
}

func (c *Controller) resolvePlaylistItems(items []string) ([]string, error) {
	resolved := make([]string, 0, len(items))
	for _, item := range items {
		path, err := c.resolvePlaylistItem(item)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

func (c *Controller) resolvePlaylistItem(item string) (string, error) {
	id, err := specid.New(item)
	if err != nil {
		return item, nil
	}
	located, err := specidpaths.Locate(c.dbc, id)
	if err != nil {
		return "", err
	}
	return located.AbsPath(), nil
}

func renderPlaylist(path string, stored *playlist.Playlist, includeItems bool) playlistResponse {
	resp := playlistResponse{ID: encodePlaylistID(path), Name: stored.Name, Comment: stored.Comment, Public: stored.IsPublic, UserID: stored.UserID, UpdatedAt: stored.UpdatedAt, SongCount: len(stored.Items)}
	if includeItems {
		resp.Items = stored.Items
	}
	return resp
}
