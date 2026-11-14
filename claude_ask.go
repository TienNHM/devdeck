package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// POST /api/claude/ask runs `claude -p` once and streams its stdout to the browser as NDJSON:
//
//	{"t":"text chunk"} ... then {"done":true} or {"err":"message"}
//
// The prompt and context go to stdin (never to the command line, so nothing can be read as a flag
// and Windows .cmd shims never re-parse it). The only argument is -p: no permission-bypass flags.
// Closing the response (Cancel in the UI) cancels the request context, which kills the process tree.

const (
	askTimeout    = 5 * time.Minute
	askOutputCap  = 512 << 10
	askPromptMax  = 32 << 10
	askContextMax = 400 << 10
)

var askSlots = make(chan struct{}, 2)

func claudeBin() string {
	if b := strings.TrimSpace(os.Getenv("DEVDECK_CLAUDE_BIN")); b != "" {
		return b
	}
	return "claude"
}

type askReq struct {
	Prompt  string `json:"prompt"`
	Context string `json:"context"`
	Cwd     string `json:"cwd"`
}

func askCwd(c string) (string, error) {
	if strings.TrimSpace(c) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	return cleanRepo(strings.TrimSpace(c)) // existing directory, absolute, no leading dash
}

func askInput(r askReq) string {
	if strings.TrimSpace(r.Context) == "" {
		return r.Prompt
	}
	return r.Prompt + "\n\n<context>\n" + r.Context + "\n</context>\n"
}

// utf8Cut returns the longest prefix of b that does not end inside a multi-byte rune.
func utf8Cut(b []byte) int {
	n := len(b)
	for i := 1; i <= 3 && i <= n; i++ {
		c := b[n-i]
		if c < 0x80 { // ASCII: complete
			return n
		}
		if c >= 0xC0 { // lead byte: complete only if the whole rune is present
			if utf8.FullRune(b[n-i:]) {
				return n
			}
			return n - i
		}
	}
	return n
}

func handleClaudeAsk(w http.ResponseWriter, r *http.Request) {
	var req askReq
	body := http.MaxBytesReader(w, r.Body, askContextMax+askPromptMax+4096)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		fail(w, 400, errors.New("request too large or invalid"))
		return
	}
	// Read the body to EOF: only then does net/http watch the connection and cancel r.Context()
	// when the browser goes away (Cancel button), which is what kills the process.
	_, _ = io.Copy(io.Discard, body)
	if strings.TrimSpace(req.Prompt) == "" {
		fail(w, 400, errors.New("prompt is empty"))
		return
	}
	if len(req.Prompt) > askPromptMax || len(req.Context) > askContextMax {
		fail(w, 400, errors.New("prompt or context too long"))
		return
	}
	cwd, err := askCwd(req.Cwd)
	if err != nil {
		fail(w, 400, err)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, errors.New("streaming unsupported"))
		return
	}
	select {
	case askSlots <- struct{}{}:
		defer func() { <-askSlots }()
	default:
		fail(w, 429, errors.New("two Claude requests are already running; wait or cancel one"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), askTimeout)
	defer cancel()
	cmd := exec.Command(claudeBin(), "-p")
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(askInput(req))
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(w, 500, err)
		return
	}
	stderr := &capBuf{max: 8 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		fail(w, 500, fmt.Errorf("cannot start %s: %v (set DEVDECK_CLAUDE_BIN if claude is not on PATH)", claudeBin(), err))
		return
	}
	// Kill the whole tree (claude may be a node process behind a shim) when the client goes away or time runs out.
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = killProcess(int32(cmd.Process.Pid), true)
			_ = cmd.Process.Kill()
			// a surviving grandchild could keep the pipe open: unblock the reader after a grace period
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				_ = stdout.Close()
			}
		case <-stopped:
		}
	}()

	h := w.Header()
	h.Set("Content-Type", "application/x-ndjson; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	enc := json.NewEncoder(w)
	send := func(v any) bool {
		if enc.Encode(v) != nil {
			return false
		}
		fl.Flush()
		return true
	}

	buf := make([]byte, 8<<10)
	var pend []byte
	total, capped, clientGone := 0, false, false
	for {
		n, rerr := stdout.Read(buf)
		if n > 0 && !capped && !clientGone {
			if total+n > askOutputCap {
				n = askOutputCap - total
				capped = true
			}
			total += n
			pend = append(pend, buf[:n]...)
			cut := utf8Cut(pend)
			if capped {
				cut = len(pend)
			}
			if cut > 0 {
				if !send(map[string]string{"t": strings.ToValidUTF8(string(pend[:cut]), "�")}) {
					clientGone = true
				}
				pend = append(pend[:0], pend[cut:]...)
			}
			if capped || clientGone {
				cancel() // stop the process; keep draining so Wait returns
			}
		}
		if rerr != nil {
			break
		}
	}
	if len(pend) > 0 && !clientGone {
		send(map[string]string{"t": strings.ToValidUTF8(string(pend), "�")})
	}
	werr := cmd.Wait()
	close(stopped)
	if clientGone {
		return
	}
	switch {
	case capped:
		send(map[string]string{"err": fmt.Sprintf("output limit reached (%d KB); the rest was cut", askOutputCap>>10)})
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		send(map[string]string{"err": "timed out after 5 minutes"})
	case r.Context().Err() != nil:
		// cancelled by the browser: nobody is listening
	case werr != nil:
		msg := strings.TrimSpace(stderr.buf.String())
		if msg == "" {
			msg = werr.Error()
		}
		send(map[string]string{"err": "claude failed: " + msg})
	default:
		send(map[string]bool{"done": true})
	}
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/claude/ask", handleClaudeAsk)
	})
}
