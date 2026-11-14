package main

// Server-sent events helpers shared by the log tailer and docker container logs.
//
// Stream protocol (consumed by logsViewer in web/logs.js):
//
//	data: ["line", "line", ...]        a batch of log lines
//	event: info  / data: "text"        a notice (truncated, rotated, skipped bytes…)
//	event: fail  / data: "text"        an error; the stream ends
//	event: end   / data: "text"        the stream ended normally (container exited, cap reached)

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	logMaxLine     = 16 << 10 // longer lines are cut
	sseBatchLines  = 500
	streamCapBytes = 64 << 20 // a single follow stream ends after this much data
)

type sseStream struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSE(w http.ResponseWriter) (*sseStream, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sseStream{w: w, f: f}, nil
}

func (s *sseStream) send(event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: " + event + "\n")
	}
	sb.WriteString("data: ")
	sb.Write(b)
	sb.WriteString("\n\n")
	if _, err := io.WriteString(s.w, sb.String()); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

func (s *sseStream) lines(ls []string) error {
	for len(ls) > 0 {
		n := min(len(ls), sseBatchLines*2)
		if err := s.send("", ls[:n]); err != nil {
			return err
		}
		ls = ls[n:]
	}
	return nil
}

func (s *sseStream) ping() error {
	if _, err := io.WriteString(s.w, ": ping\n\n"); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// cleanLine strips ANSI escapes and CR, cuts very long lines and keeps the text valid UTF-8.
func cleanLine(b []byte) string {
	if len(b) > logMaxLine {
		b = b[:logMaxLine]
	}
	s := strings.TrimRight(string(b), "\r\n")
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = ansiRe.ReplaceAllString(s, "")
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return s
}

// readLines calls emit for every line read from r (long lines are cut, not split). It stops when emit returns false.
func readLines(r io.Reader, emit func(string) bool) error {
	br := bufio.NewReaderSize(r, 32<<10)
	var long []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			if len(long) < logMaxLine {
				long = append(long, chunk...)
			}
			continue
		}
		if len(long) > 0 {
			chunk = append(long, chunk...)
			long = nil
		}
		if len(chunk) > 0 {
			if !emit(cleanLine(chunk)) {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
