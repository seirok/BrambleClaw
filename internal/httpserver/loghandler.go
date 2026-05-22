package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ClientLogEntry represents a frontend error log entry
type ClientLogEntry struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Stack     string `json:"stack,omitempty"`
	URL       string `json:"url,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Browser   string `json:"browser,omitempty"`
	Page      string `json:"page,omitempty"`
}

// clientLogMu protects concurrent writes to log files
var clientLogMu sync.Mutex

// handleClientLog handles POST /api/log
func (s *Server) handleClientLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed",
		})
		return
	}

	var entry ClientLogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		s.jsonResponse(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON body",
		})
		return
	}

	if entry.Message == "" {
		s.jsonResponse(w, http.StatusBadRequest, map[string]string{
			"error": "message is required",
		})
		return
	}

	// Write to date-rotated log file
	logDir := filepath.Join(".", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		s.jsonResponse(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create log directory",
		})
		return
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("app-%s.log", time.Now().Format("2006-01-02")))

	clientLogMu.Lock()
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		clientLogMu.Unlock()
		s.jsonResponse(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to open log file",
		})
		return
	}

	line, _ := json.Marshal(entry)
	_, writeErr := f.Write(append(line, '\n'))
	closeErr := f.Close()
	clientLogMu.Unlock()

	if writeErr != nil || closeErr != nil {
		err := writeErr
		if err == nil {
			err = closeErr
		}
		s.jsonResponse(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to write log",
		})
		return
	}

	s.jsonResponse(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}
