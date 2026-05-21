package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"go.senan.xyz/gonic"
	"go.senan.xyz/gonic/db"
	"go.senan.xyz/gonic/scanner"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type scanResponse struct {
	Scanning     bool   `json:"scanning"`
	TrackCount   int    `json:"trackCount"`
	LastScanTime string `json:"lastScanTime,omitempty"`
}

func (c *Controller) serveHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: gonic.Version})
}

func (c *Controller) serveScan(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	go c.scanLibrary()
	c.writeScanStatus(w)
}

func (c *Controller) serveScanStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	c.writeScanStatus(w)
}

func (c *Controller) scanLibrary() {
	if _, err := c.scanner.ScanAndClean(scanner.ScanOptions{}); err != nil && !errors.Is(err, scanner.ErrAlreadyScanning) {
		log.Printf("api: scan library: %v", err)
	}
}

func (c *Controller) writeScanStatus(w http.ResponseWriter) {
	status, err := c.scanStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scan status: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (c *Controller) scanStatus() (scanResponse, error) {
	var trackCount int
	if err := c.dbc.Model(db.Track{}).Count(&trackCount).Error; err != nil {
		return scanResponse{}, err
	}
	lastScan, _ := c.dbc.GetSetting(db.LastScanTime)
	return scanResponse{Scanning: c.scanner.IsScanning(), TrackCount: trackCount, LastScanTime: formatScanTime(lastScan)}, nil
}

func formatScanTime(value string) string {
	if value == "" {
		return ""
	}
	sec, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return value
	}
	return time.Unix(sec, 0).Format(time.RFC3339)
}

func (c *Controller) serveNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}
