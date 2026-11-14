package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/coder/websocket"
)

type termInit struct {
	Kind       string `json:"kind"` // "local" | "ssh"
	Title      string `json:"title"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	Cwd        string `json:"cwd"`
	Cmd        string `json:"cmd"`     // optional command to run (e.g. "claude")
	Shell      string `json:"shell"`   // local shell id from /api/term/shells (empty = default)
	Profile    string `json:"profile"` // saved or ~/.ssh/config host name
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	KeyPath    string `json:"keyPath"`
	Password   string `json:"password"`
	Passphrase string `json:"passphrase"`
	Attach     string `json:"attach"` // re-attach to a live session instead of starting one
}

type ptyLike interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// termShell is a local shell offered in the picker.
type termShell struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"-"`
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/term/sessions", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, termReg.list()) })
		mux.HandleFunc("DELETE /api/term/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			if s := termReg.get(r.PathValue("id")); s != nil {
				s.kill()
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("GET /api/term/shells", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, localShells()) })
	})
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// handleTermWS speaks a tiny protocol: the first text message is a termInit (start a session) or
// {"attach": id}. Then binary frames carry terminal bytes both ways; text frames are control
// messages: server→client {"id":…}, {"exit":true}, {"error":…,"auth":bool}; client→server
// {"resize":{cols,rows}} and {"close":true} (kill). Closing the socket only detaches.
func handleTermWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil) // enforces same-origin
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	_, data, err := ws.Read(ctx)
	if err != nil {
		return
	}
	var in termInit
	if err := json.Unmarshal(data, &in); err != nil {
		ws.Close(websocket.StatusPolicyViolation, "bad init message")
		return
	}
	control := func(v any) {
		b, _ := json.Marshal(v)
		_ = ws.Write(ctx, websocket.MessageText, b)
	}

	var s *termSession
	if in.Attach != "" {
		if s = termReg.get(in.Attach); s == nil {
			control(map[string]any{"error": "session has ended", "gone": true})
			ws.Close(websocket.StatusNormalClosure, "gone")
			return
		}
	} else {
		if s, err = startTermSession(in); err != nil {
			control(map[string]any{"error": err.Error(), "auth": errors.Is(err, errSSHAuth)})
			ws.Close(websocket.StatusNormalClosure, "failed")
			return
		}
		control(map[string]any{"id": s.ID, "title": s.Title, "kind": s.Kind, "where": s.Where})
	}

	c, replay, err := s.attach()
	if err != nil {
		control(map[string]any{"error": err.Error(), "gone": true})
		ws.Close(websocket.StatusNormalClosure, "gone")
		return
	}
	s.resize(in.Cols, in.Rows)
	if in.Attach != "" {
		control(map[string]any{"id": s.ID, "title": s.Title, "kind": s.Kind, "where": s.Where, "replay": len(replay)})
	}
	if len(replay) > 0 {
		if err := ws.Write(ctx, websocket.MessageBinary, replay); err != nil {
			s.detach(c)
			return
		}
	}

	var once sync.Once
	stop := func() { once.Do(cancel) }
	done := make(chan struct{})
	go func() { // session → browser
		defer close(done)
		defer stop()
		for chunk := range c.ch {
			if err := ws.Write(ctx, websocket.MessageBinary, chunk); err != nil {
				return
			}
		}
		s.mu.Lock()
		exited := s.exited
		s.mu.Unlock()
		if exited {
			control(map[string]any{"exit": true})
		}
	}()
	go func() { // browser → session
		defer stop()
		for {
			typ, msg, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				if err := s.write(msg); err != nil {
					return
				}
				continue
			}
			var m struct {
				Resize *struct{ Cols, Rows int } `json:"resize"`
				Close  bool                      `json:"close"`
			}
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			if m.Close {
				s.kill()
				return
			}
			if m.Resize != nil {
				s.resize(m.Resize.Cols, m.Resize.Rows)
			}
		}
	}()
	<-ctx.Done()
	s.detach(c)
	<-done
	ws.Close(websocket.StatusNormalClosure, "bye")
}

// startTermSession starts a local shell or SSH session described by in. Connection messages
// (e.g. a host key being trusted) are put at the start of the replay buffer.
func startTermSession(in termInit) (*termSession, error) {
	if termReg.full() {
		return nil, errTermFull
	}
	if in.Cols <= 0 || in.Cols > 1000 {
		in.Cols = 120
	}
	if in.Rows <= 0 || in.Rows > 500 {
		in.Rows = 30
	}
	if len(in.Cmd) > 4096 || strings.ContainsAny(in.Cmd, "\r\n\x00") {
		return nil, errors.New("command must be a single line")
	}
	var lead []byte // shown through the replay right after the session starts
	say := func(msg string) { lead = append(lead, "\x1b[33m"+msg+"\x1b[0m\r\n"...) }
	title := clip(strings.TrimSpace(in.Title), 80)
	var (
		p     ptyLike
		where string
		err   error
	)
	switch in.Kind {
	case "local":
		p, err = startLocal(in)
		where = in.Cwd
		if title == "" {
			title = "shell"
		}
	case "ssh":
		var t sshTarget
		if t, err = targetFromInit(in); err == nil {
			p, err = startSSH(t, in, say)
			where = t.label()
		}
		if title == "" {
			title = "ssh · " + where
		}
	default:
		err = errors.New("unknown terminal kind")
	}
	if err != nil {
		return nil, err
	}
	s, err := termReg.start(title, in.Kind, where, p, lead)
	if err != nil {
		p.Close()
		return nil, err
	}
	s.wmu.Lock()
	s.Cols, s.Rows = in.Cols, in.Rows
	s.wmu.Unlock()
	return s, nil
}
