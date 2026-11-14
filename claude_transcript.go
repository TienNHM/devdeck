package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Transcript details are read lazily, only when the UI asks for one session, from the tail of
// the JSONL file (at most transcriptTail bytes) plus its first line for the start time.

const (
	transcriptTail = 2 << 20
	snippetMax     = 1200
)

type TranscriptInfo struct {
	LastPrompt  string `json:"lastPrompt,omitempty"`
	LastReply   string `json:"lastReply,omitempty"`
	Model       string `json:"model,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Version     string `json:"version,omitempty"`
	InputTokens int64  `json:"inputTokens"`
	OutputToks  int64  `json:"outputTokens"`
	CacheRead   int64  `json:"cacheReadTokens"`
	CacheWrite  int64  `json:"cacheWriteTokens"`
	Started     int64  `json:"started,omitempty"` // unix seconds of the first entry
	Last        int64  `json:"last,omitempty"`    // unix seconds of the last entry
	Messages    int    `json:"messages"`
	Partial     bool   `json:"partial,omitempty"` // only the tail was read; token totals cover that part
	Size        int64  `json:"size"`
}

type tUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type tLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	IsMeta    bool   `json:"isMeta"`
	GitBranch string `json:"gitBranch"`
	Version   string `json:"version"`
	Message   *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *tUsage         `json:"usage"`
	} `json:"message"`
}

type tBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func parseTS(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

// userText returns the typed prompt of a user entry, or "" for tool results and command wrappers.
func userText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return promptOnly(s)
	}
	var blocks []tBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return ""
		}
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return promptOnly(strings.Join(parts, "\n"))
}

func promptOnly(s string) string {
	t := strings.TrimSpace(s)
	for _, p := range []string{"<command-", "<local-command-", "<system-reminder>", "Caveat: ", "[Request interrupted"} {
		if strings.HasPrefix(t, p) {
			return ""
		}
	}
	return t
}

func assistantText(raw json.RawMessage) string {
	var blocks []tBlock
	if json.Unmarshal(raw, &blocks) != nil {
		var s string
		_ = json.Unmarshal(raw, &s)
		return strings.TrimSpace(s)
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// parseTranscript reads the tail of a Claude Code JSONL transcript.
func parseTranscript(path string) (*TranscriptInfo, error) {
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
		return nil, errors.New("transcript is not a regular file")
	}
	info := &TranscriptInfo{Size: st.Size()}

	// Start time: first timestamped entry within the first 64 KB.
	head := bufio.NewScanner(io.LimitReader(f, 64<<10))
	head.Buffer(make([]byte, 0, 64<<10), 64<<10)
	for head.Scan() {
		var l tLine
		if json.Unmarshal(head.Bytes(), &l) == nil {
			if ts := parseTS(l.Timestamp); ts > 0 {
				info.Started = ts
				break
			}
		}
	}

	off := int64(0)
	if st.Size() > transcriptTail {
		off = st.Size() - transcriptTail
		info.Partial = true
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(io.LimitReader(f, transcriptTail))
	if err != nil {
		return nil, err
	}
	if off > 0 { // drop the partial first line
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			buf = nil
		}
	}
	usage := map[string]tUsage{} // assistant messages are split over several lines with repeated usage
	anon := 0
	for len(buf) > 0 {
		line := buf
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			line, buf = buf[:i], buf[i+1:]
		} else {
			buf = nil
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var l tLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		if ts := parseTS(l.Timestamp); ts > 0 {
			info.Last = ts
			if info.Started == 0 {
				info.Started = ts
			}
		}
		if l.GitBranch != "" {
			info.Branch = l.GitBranch
		}
		if l.Version != "" {
			info.Version = l.Version
		}
		if l.Message == nil {
			continue
		}
		switch l.Type {
		case "user":
			if l.IsMeta {
				continue
			}
			if t := userText(l.Message.Content); t != "" {
				info.LastPrompt = clip(t, snippetMax)
				info.Messages++
			}
		case "assistant":
			if m := l.Message.Model; m != "" && !strings.HasPrefix(m, "<") {
				info.Model = m
			}
			if t := assistantText(l.Message.Content); t != "" {
				info.LastReply = clip(t, snippetMax)
			}
			if l.Message.Usage != nil {
				k := l.Message.ID
				if k == "" {
					anon++
					k = "#" + strconv.Itoa(anon)
				}
				usage[k] = *l.Message.Usage
			}
		}
	}
	for _, u := range usage {
		info.InputTokens += u.InputTokens
		info.OutputToks += u.OutputTokens
		info.CacheRead += u.CacheReadInputTokens
		info.CacheWrite += u.CacheCreationInputTokens
	}
	return info, nil
}

// A tiny cache so re-opening the same card does not re-read an unchanged file.
var transcriptCache = struct {
	sync.Mutex
	m map[string]tcEntry
}{m: map[string]tcEntry{}}

type tcEntry struct {
	size int64
	mod  time.Time
	info *TranscriptInfo
}

func transcriptInfo(path string) (*TranscriptInfo, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return nil, errors.New("unexpected transcript path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, errors.New("transcript not found")
	}
	transcriptCache.Lock()
	e, ok := transcriptCache.m[path]
	transcriptCache.Unlock()
	if ok && e.size == st.Size() && e.mod.Equal(st.ModTime()) {
		return e.info, nil
	}
	info, err := parseTranscript(path)
	if err != nil {
		return nil, err
	}
	transcriptCache.Lock()
	if len(transcriptCache.m) >= 32 {
		transcriptCache.m = map[string]tcEntry{}
	}
	transcriptCache.m[path] = tcEntry{size: st.Size(), mod: st.ModTime(), info: info}
	transcriptCache.Unlock()
	return info, nil
}

// ------------------------------------------------------------ hooks status (read-only)

type HookStatus struct {
	SettingsPath string   `json:"settingsPath"`
	Found        bool     `json:"found"`     // settings.json exists
	Installed    bool     `json:"installed"` // at least one devdeck hook
	Events       []string `json:"events"`    // events with a devdeck hook
	Missing      []string `json:"missing"`   // recommended events without one
	Error        string   `json:"error,omitempty"`
	Snippet      string   `json:"snippet"`
}

var recommendedHookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Notification", "Stop", "SessionEnd"}

// claudeHookStatus reads ~/.claude/settings.json without ever writing to it.
func claudeHookStatus() HookStatus {
	hs := HookStatus{Events: []string{}, Missing: []string{}, Snippet: hooksConfigText()}
	home, err := os.UserHomeDir()
	if err != nil {
		hs.Error = err.Error()
		return hs
	}
	hs.SettingsPath = filepath.Join(home, ".claude", "settings.json")
	f, err := os.Open(hs.SettingsPath)
	if err != nil {
		hs.Missing = append(hs.Missing, recommendedHookEvents...)
		return hs
	}
	defer f.Close()
	hs.Found = true
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&cfg); err != nil {
		hs.Error = "cannot parse settings.json: " + err.Error()
	}
	has := map[string]bool{}
	for ev, groups := range cfg.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if strings.Contains(strings.ToLower(h.Command), "devdeck") {
					has[ev] = true
				}
			}
		}
	}
	for _, ev := range recommendedHookEvents {
		if has[ev] {
			hs.Events = append(hs.Events, ev)
		} else {
			hs.Missing = append(hs.Missing, ev)
		}
	}
	hs.Installed = len(hs.Events) > 0
	return hs
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		// Richer, lazily loaded details for one session (transcript tail).
		mux.HandleFunc("GET /api/claude/session", func(w http.ResponseWriter, r *http.Request) {
			s, ok := getClaudeSession(r.URL.Query().Get("id"))
			if !ok {
				fail(w, 404, errors.New("unknown session"))
				return
			}
			res := map[string]any{"session": s}
			if s.Transcript == "" {
				res["error"] = "no transcript path reported yet"
			} else if ti, err := transcriptInfo(s.Transcript); err != nil {
				res["error"] = err.Error()
			} else {
				res["transcript"] = ti
			}
			writeJSON(w, res)
		})
		mux.HandleFunc("GET /api/claude/setup", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, claudeHookStatus())
		})
	})
}
