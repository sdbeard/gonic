package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.senan.xyz/gonic/db"
	"go.senan.xyz/gonic/server/ctrlsubsonic/specid"
	"go.senan.xyz/gonic/server/ctrlsubsonic/specidpaths"
	"go.senan.xyz/gonic/transcode"
)

func (c *Controller) serveStream(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	file, audioFile, ok := c.locateAudio(w, r)
	if !ok {
		return
	}
	profile, ok, err := streamProfile(r, audioFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "stream profile: %v", err)
		return
	}
	if !ok {
		http.ServeFile(w, r, file.AbsPath())
		return
	}
	w.Header().Set("Content-Type", profile.MIME())
	if err := c.transcoder.Transcode(r.Context(), profile, file.AbsPath(), w); err != nil && !errors.Is(err, transcode.ErrFFmpegKilled) {
		writeError(w, http.StatusInternalServerError, "transcode: %v", err)
	}
}

func (c *Controller) locateAudio(w http.ResponseWriter, r *http.Request) (specidpaths.Result, db.AudioFile, bool) {
	id, err := specid.New(strings.TrimPrefix(r.URL.Path, "/stream/"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad track id: %v", err)
		return nil, nil, false
	}
	file, err := specidpaths.Locate(c.dbc, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "track %s not found", id.String())
		return nil, nil, false
	}
	audioFile, ok := file.(db.AudioFile)
	if !ok {
		writeError(w, http.StatusBadRequest, "id %s is not audio", id.String())
		return nil, nil, false
	}
	return file, audioFile, true
}

func streamProfile(r *http.Request, audioFile db.AudioFile) (transcode.Profile, bool, error) {
	format := r.URL.Query().Get("format")
	profileName := r.URL.Query().Get("profile")
	if format == "raw" || profileName == "" {
		return transcode.Profile{}, false, nil
	}
	profile, ok := transcode.UserProfiles[profileName]
	if !ok {
		return transcode.Profile{}, false, fmt.Errorf("unknown profile %q", profileName)
	}
	profile = withRequestedBitrate(r, audioFile, profile)
	return withRequestedSeek(r, profile), true, nil
}

func withRequestedBitrate(r *http.Request, audioFile db.AudioFile, profile transcode.Profile) transcode.Profile {
	maxBitRate, _ := strconv.Atoi(r.URL.Query().Get("maxBitRate"))
	if maxBitRate == 0 || maxBitRate >= audioFile.AudioBitrate() || int(profile.BitRate()) <= maxBitRate {
		return profile
	}
	return transcode.WithBitrate(profile, transcode.BitRate(maxBitRate))
}

func withRequestedSeek(r *http.Request, profile transcode.Profile) transcode.Profile {
	offset, _ := strconv.Atoi(r.URL.Query().Get("timeOffset"))
	if offset == 0 {
		return profile
	}
	return transcode.WithSeek(profile, time.Second*time.Duration(offset))
}
