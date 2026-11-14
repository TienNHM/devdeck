package main

// Logs tab backend: a saved list of log files (~/.devdeck/logs.json), tail of the last N lines
// and a follow stream that checks the file about once a second only while a viewer is connected.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	logsMaxFiles    = 100
	logsMaxTail     = 5000
	logsTailBytes   = 8 << 20 // never read more than this backwards for a tail
	logsFollowChunk = 4 << 20 // at most this much new data per check; more is skipped
)

type logsStore struct {
	Files []string `json:"files"`
}

var logsMu sync.Mutex

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func logsList() []string {
	var s logsStore
	_ = loadJSON("logs", &s)
	if s.Files == nil {
		s.Files = []string{}
	}
	return s.Files
}

// checkLogFile normalizes p to an absolute path and requires an existing regular file.
func checkLogFile(p string) (string, os.FileInfo, error) {
	p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`))
	if p == "" {
		return "", nil, errors.New("path is required")
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimLeft(p[1:], `/\`))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, errors.New("file not found: " + abs)
		}
		return "", nil, err
	}
	if !st.Mode().IsRegular() {
		return "", nil, errors.New("not a regular file: " + abs)
	}
	return abs, st, nil
}

func logsAdd(p string) ([]string, error) {
	abs, _, err := checkLogFile(p)
	if err != nil {
		return nil, err
	}
	logsMu.Lock()
	defer logsMu.Unlock()
	files := logsList()
	for _, f := range files {
		if samePath(f, abs) {
			return files, nil
		}
	}
	if len(files) >= logsMaxFiles {
		return nil, fmt.Errorf("at most %d log files", logsMaxFiles)
	}
	files = append(files, abs)
	return files, saveJSON("logs", logsStore{Files: files})
}

func logsRemove(p string) ([]string, error) {
	logsMu.Lock()
	defer logsMu.Unlock()
	files := logsList()
	out := files[:0]
	for _, f := range files {
		if !samePath(f, p) {
			out = append(out, f)
		}
	}
	return out, saveJSON("logs", logsStore{Files: out})
}

// logsAllowed returns the saved path matching p; the HTTP API only reads files the user added.
func logsAllowed(p string) (string, error) {
	for _, f := range logsList() {
		if samePath(f, p) {
			return f, nil
		}
	}
	return "", errors.New("not in the saved log files: " + p)
}

type logFileInfo struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Error string `json:"error,omitempty"`
}

func logsInfo() []logFileInfo {
	files := logsList()
	res := make([]logFileInfo, 0, len(files))
	for _, f := range files {
		i := logFileInfo{Path: f, Name: filepath.Base(f)}
		if st, err := os.Stat(f); err != nil {
			i.Error = "missing"
		} else if !st.Mode().IsRegular() {
			i.Error = "not a regular file"
		} else {
			i.Size, i.MTime = st.Size(), st.ModTime().Unix()
		}
		res = append(res, i)
	}
	return res
}

// tailFile returns up to n lines that end before `end` (the file size at the time of the call).
// When complete is true, a trailing line without '\n' is left out and `next` points at its start,
// so a follow loop can pick it up once it is finished.
func tailFile(f *os.File, end int64, n int, complete bool) (lines []string, next int64, err error) {
	const chunk = 64 << 10
	pos := end
	var buf []byte
	nl := 0
	for pos > 0 && nl <= n && end-pos < logsTailBytes {
		sz := min(int64(chunk), pos)
		pos -= sz
		b := make([]byte, sz)
		if _, err := f.ReadAt(b, pos); err != nil && !errors.Is(err, io.EOF) {
			return nil, 0, err
		}
		nl += bytes.Count(b, []byte{'\n'})
		buf = append(b, buf...)
	}
	next = end
	if complete {
		if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
			next = pos + int64(i) + 1
			buf = buf[:i+1]
		} else if pos == 0 {
			return []string{}, 0, nil
		} else {
			return []string{}, end, nil // one giant line: skip it
		}
	}
	if pos > 0 { // the first line is partial
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			buf = nil
		}
	}
	text := strings.TrimSuffix(string(buf), "\n")
	if text == "" && len(buf) == 0 {
		return []string{}, next, nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	for i, p := range parts {
		parts[i] = cleanLine([]byte(p))
	}
	return parts, next, nil
}

// tailPath reads the last n lines of a file. With a non-nil filter, it returns the last n matching
// lines found in the last logsTailBytes of the file.
func tailPath(path string, n int, filter *regexp.Regexp) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if filter == nil {
		lines, _, err := tailFile(f, st.Size(), n, false)
		return lines, err
	}
	all, _, err := tailFile(f, st.Size(), 1<<30, false)
	if err != nil {
		return nil, err
	}
	var res []string
	for _, l := range all {
		if filter.MatchString(l) {
			res = append(res, l)
		}
	}
	if len(res) > n {
		res = res[len(res)-n:]
	}
	return res, nil
}

// statID stats p and pins its file identity (Windows loads it lazily from the path, which would be wrong after a rotation).
func statID(p string) (os.FileInfo, error) {
	st, err := os.Stat(p)
	if err == nil {
		os.SameFile(st, st)
	}
	return st, err
}

type followEvents struct {
	lines func([]string) error
	info  func(string) error
}

// followFile tails path from offset until ctx ends, checking it every `every`. It handles truncation,
// rotation (a new file at the same path) and a file that disappears and comes back.
func followFile(ctx context.Context, path string, prev os.FileInfo, offset int64, every time.Duration, ev followEvents) error {
	tick := time.NewTicker(every)
	defer tick.Stop()
	var pending []byte
	stale, idle := 0, 0
	missing := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		st, err := statID(path)
		if err != nil || !st.Mode().IsRegular() {
			if !missing {
				missing = true
				if err := ev.info("file is missing, waiting for it to come back"); err != nil {
					return err
				}
			}
			continue
		}
		switch {
		case missing || !os.SameFile(prev, st):
			msg := "file was replaced (rotated), reading the new file"
			if missing {
				msg = "file is back, reading from the start"
			}
			missing, offset, pending = false, 0, nil
			if err := ev.info(msg); err != nil {
				return err
			}
		case st.Size() < offset:
			offset, pending = 0, nil
			if err := ev.info("file was truncated, reading from the start"); err != nil {
				return err
			}
		}
		prev = st
		size := st.Size()
		if size <= offset {
			if len(pending) > 0 {
				if stale++; stale >= 2 { // an unfinished last line: show it after ~2 checks
					if err := ev.lines([]string{cleanLine(pending)}); err != nil {
						return err
					}
					pending, stale = nil, 0
				}
			} else if idle++; idle >= 15 {
				idle = 0
				if err := ev.lines(nil); err != nil { // keep-alive
					return err
				}
			}
			continue
		}
		idle, stale = 0, 0
		if size-offset > logsFollowChunk {
			skipped := size - logsFollowChunk - offset
			offset, pending = size-logsFollowChunk, nil
			if err := ev.info(fmt.Sprintf("skipped %d bytes written too fast", skipped)); err != nil {
				return err
			}
			// drop the partial first line
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			b := make([]byte, 64<<10)
			k, _ := f.ReadAt(b, offset)
			f.Close()
			if i := bytes.IndexByte(b[:k], '\n'); i >= 0 {
				offset += int64(i) + 1
			}
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		data := make([]byte, size-offset)
		k, rerr := f.ReadAt(data, offset)
		f.Close()
		data = data[:k]
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			continue
		}
		offset += int64(k)
		data = append(pending, data...)
		pending = nil
		last := bytes.LastIndexByte(data, '\n')
		if last < 0 {
			pending = data
			if len(pending) > logMaxLine {
				pending = pending[:logMaxLine]
			}
			continue
		}
		if last+1 < len(data) {
			pending = append([]byte(nil), data[last+1:]...)
		}
		parts := strings.Split(string(data[:last]), "\n")
		lines := make([]string, len(parts))
		for i, p := range parts {
			lines[i] = cleanLine([]byte(p))
		}
		if err := ev.lines(lines); err != nil {
			return err
		}
	}
}

func logsFollowHandler(w http.ResponseWriter, r *http.Request) {
	s, err := newSSE(w)
	if err != nil {
		fail(w, 500, err)
		return
	}
	q := r.URL.Query()
	path, err := logsAllowed(q.Get("path"))
	if err != nil {
		_ = s.send("fail", err.Error())
		return
	}
	n := clampInt(atoi(q.Get("lines")), 1, logsMaxTail, 200)
	st, err := statID(path)
	if err != nil || !st.Mode().IsRegular() {
		_ = s.send("fail", "cannot open "+path)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		_ = s.send("fail", err.Error())
		return
	}
	lines, offset, err := tailFile(f, st.Size(), n, true)
	f.Close()
	if err != nil {
		_ = s.send("fail", err.Error())
		return
	}
	if s.lines(lines) != nil {
		return
	}
	total := 0
	err = followFile(r.Context(), path, st, offset, time.Second, followEvents{
		lines: func(ls []string) error {
			if ls == nil {
				return s.ping()
			}
			for _, l := range ls {
				total += len(l) + 1
			}
			if total > streamCapBytes {
				_ = s.send("end", "stopped: stream limit reached, follow again to continue")
				return errors.New("cap")
			}
			return s.lines(ls)
		},
		info: func(m string) error { return s.send("info", m) },
	})
	_ = err
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/logs/files", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, logsInfo()) })
		mux.HandleFunc("POST /api/logs/files", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Path string `json:"path"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			if _, err := logsAdd(b.Path); err != nil {
				fail(w, 400, err)
				return
			}
			writeJSON(w, logsInfo())
		})
		mux.HandleFunc("POST /api/logs/files/remove", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Path string `json:"path"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			if _, err := logsRemove(b.Path); err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, logsInfo())
		})
		mux.HandleFunc("GET /api/logs/tail", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			path, err := logsAllowed(q.Get("path"))
			if err != nil {
				fail(w, 400, err)
				return
			}
			lines, err := tailPath(path, clampInt(atoi(q.Get("lines")), 1, logsMaxTail, 200), nil)
			if err != nil {
				fail(w, 400, err)
				return
			}
			writeJSON(w, map[string]any{"lines": lines})
		})
		mux.HandleFunc("GET /api/logs/follow", logsFollowHandler)
	})

	registerMCPTool(tool{Name: "tail_file", Description: "Last lines of a text/log file, optionally only lines matching grep (RE2 regex; plain text if it is not a valid regex). Max 2000 lines; with grep only the last 8 MB are searched.",
		InputSchema: obj(map[string]any{"path": typ("string", "file path"), "lines": typ("integer", "number of lines, default 100, max 2000"), "grep": typ("string", "optional filter")}, "path"),
		handler: func(a map[string]any) (string, error) {
			path, _, err := checkLogFile(argStr(a, "path", ""))
			if err != nil {
				return "", err
			}
			var re *regexp.Regexp
			if g := argStr(a, "grep", ""); g != "" {
				if re, err = regexp.Compile(g); err != nil {
					re = regexp.MustCompile(regexp.QuoteMeta(g))
				}
			}
			lines, err := tailPath(path, clampInt(argInt(a, "lines", 100), 1, 2000, 100), re)
			if err != nil {
				return "", err
			}
			if len(lines) == 0 {
				return "(no lines)\n", nil
			}
			return strings.Join(lines, "\n") + "\n", nil
		}})
}
