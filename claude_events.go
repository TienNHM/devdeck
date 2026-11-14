package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// claudeHub fans session changes out to Server-Sent-Events subscribers (one per open devdeck tab).
// Each subscriber gets a small buffered channel; a slow client only loses intermediate snapshots,
// and every message carries the full session list, so the next one repairs the view.

type claudeMsg struct {
	Sessions []ClaudeSession `json:"sessions"`
	Alert    *ClaudeAlert    `json:"alert,omitempty"`
}

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

var claudeHub = &hub{subs: map[chan []byte]struct{}{}}

const maxClaudeSubscribers = 32

var errTooManySubs = errors.New("too many event listeners")

func (h *hub) subscribe() (chan []byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) >= maxClaudeSubscribers {
		return nil, errTooManySubs
	}
	c := make(chan []byte, 16)
	h.subs[c] = struct{}{}
	return c, nil
}

func (h *hub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	delete(h.subs, c)
	h.mu.Unlock()
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func snapshotMsg(alert *ClaudeAlert) []byte {
	b, _ := json.Marshal(claudeMsg{Sessions: listClaudeSessions(), Alert: alert})
	return b
}

func (h *hub) publish(alert *ClaudeAlert) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) == 0 {
		return
	}
	msg := snapshotMsg(alert)
	for c := range h.subs {
		select {
		case c <- msg:
		default:
			if alert != nil { // make room so alerts are never the message that gets dropped
				select {
				case <-c:
				default:
				}
				select {
				case c <- msg:
				default:
				}
			}
		}
	}
}

func handleClaudeEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, errors.New("streaming unsupported"))
		return
	}
	c, err := claudeHub.subscribe()
	if err != nil {
		fail(w, 503, err)
		return
	}
	defer claudeHub.unsubscribe(c)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	fmt.Fprintf(w, "retry: 5000\n\ndata: %s\n\n", snapshotMsg(nil))
	fl.Flush()
	ping := time.NewTicker(25 * time.Second) // keeps proxies happy and detects dead peers
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-c:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", m); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/claude/events", handleClaudeEvents)
		mux.HandleFunc("POST /api/claude/dismiss", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				ID string `json:"id"`
			}
			if err := decode(r, &b); err != nil || b.ID == "" {
				fail(w, 400, errors.New("missing session id"))
				return
			}
			writeJSON(w, map[string]bool{"ok": dismissClaudeSession(b.ID)})
		})
	})
}
