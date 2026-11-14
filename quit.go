package main

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func removePortFile() { _ = os.Remove(filepath.Join(dataDir(), "port")) }

// handleQuit stops the whole server. The response is flushed first, then the process exits,
// so the browser gets a clean answer. Terminals and Redis connections die with the process.
func handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		removePortFile()
		os.Exit(0)
	}()
}
