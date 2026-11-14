package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type ClaudeSession struct {
	ID         string `json:"id"`
	Cwd        string `json:"cwd"`
	State      string `json:"state"` // idle | working | waiting
	Event      string `json:"event"`
	Message    string `json:"message,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Updated    int64  `json:"updated"`
	Started    int64  `json:"started,omitempty"`    // first hook seen by devdeck (unix seconds)
	Transcript string `json:"transcript,omitempty"` // transcript_path from the hook payload
}

// ClaudeAlert is pushed to the browser when a session needs attention.
type ClaudeAlert struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // waiting | done
	Cwd     string `json:"cwd"`
	Message string `json:"message,omitempty"`
}

var claudeSessions = struct {
	sync.Mutex
	m map[string]*ClaudeSession
}{m: map[string]*ClaudeSession{}}

func recordClaudeEvent(p map[string]any) {
	str := func(k string) string { s, _ := p[k].(string); return s }
	id, event := str("session_id"), str("hook_event_name")
	if id == "" || event == "" || len(id) > 200 {
		return
	}
	var alert *ClaudeAlert
	claudeSessions.Lock()
	if event == "SessionEnd" {
		delete(claudeSessions.m, id)
	} else {
		now := time.Now().Unix()
		s := claudeSessions.m[id]
		if s == nil {
			s = &ClaudeSession{ID: id, Started: now}
			claudeSessions.m[id] = s
		}
		if cwd := str("cwd"); cwd != "" {
			s.Cwd = cwd
		}
		if tp := str("transcript_path"); tp != "" && len(tp) < 4096 {
			s.Transcript = tp
		}
		prev := s.State
		s.Event, s.Updated = event, now
		switch event {
		case "SessionStart", "Stop":
			s.State, s.Message, s.Tool = "idle", "", ""
		case "UserPromptSubmit", "PreToolUse", "PostToolUse":
			s.State, s.Message = "working", ""
			if t := str("tool_name"); t != "" {
				s.Tool = t
			}
		case "Notification":
			s.State, s.Message = "waiting", str("message")
		}
		// Alert only on real transitions: a permission/input prompt while working, or a finished turn.
		// The "waiting for your input" reminder Claude sends after a finished turn is not alerted twice.
		switch {
		case s.State == "waiting" && prev != "waiting" && prev != "idle":
			alert = &ClaudeAlert{ID: id, Kind: "waiting", Cwd: s.Cwd, Message: s.Message}
		case event == "Stop" && (prev == "working" || prev == "waiting"):
			alert = &ClaudeAlert{ID: id, Kind: "done", Cwd: s.Cwd}
		}
		cutoff := time.Now().Add(-12 * time.Hour).Unix()
		for k, v := range claudeSessions.m {
			if v.Updated < cutoff {
				delete(claudeSessions.m, k)
			}
		}
	}
	claudeSessions.Unlock()
	claudeHub.publish(alert)
}

// dismissClaudeSession removes a session from the list (it reappears on its next hook event).
func dismissClaudeSession(id string) bool {
	claudeSessions.Lock()
	_, ok := claudeSessions.m[id]
	delete(claudeSessions.m, id)
	claudeSessions.Unlock()
	if ok {
		claudeHub.publish(nil)
	}
	return ok
}

func getClaudeSession(id string) (ClaudeSession, bool) {
	claudeSessions.Lock()
	defer claudeSessions.Unlock()
	s, ok := claudeSessions.m[id]
	if !ok {
		return ClaudeSession{}, false
	}
	return *s, true
}

func listClaudeSessions() []ClaudeSession {
	claudeSessions.Lock()
	defer claudeSessions.Unlock()
	out := make([]ClaudeSession, 0, len(claudeSessions.m))
	for _, s := range claudeSessions.m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

// runHook is invoked by Claude Code. It must never block or fail the user's session.
func runHook(args []string) {
	defer os.Exit(0)
	raw, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	port := readPortFile()
	if port == 0 {
		return
	}
	var p map[string]any
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	if _, ok := p["hook_event_name"]; !ok && len(args) > 0 {
		p["hook_event_name"] = args[0]
	}
	delete(p, "tool_input") // keep payloads small; may contain secrets
	delete(p, "tool_response")
	delete(p, "prompt")
	body, _ := json.Marshal(p)
	req, err := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/hook", port), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+loadToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: time.Second}).Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// hooksConfigText is the text printed by `devdeck hooks-config` (also shown in the Claude tab).
func hooksConfigText() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "devdeck"
	}
	entry := func() []any {
		return []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": `"` + exe + `" hook`}}}}
	}
	hooks := map[string]any{}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Notification", "Stop", "SessionEnd"} {
		hooks[ev] = entry()
	}
	b, _ := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	var sb strings.Builder
	sb.WriteString("# 1) Register the MCP server with Claude Code:\n")
	fmt.Fprintf(&sb, "claude mcp add devdeck -- \"%s\" mcp\n\n", exe)
	sb.WriteString("# 2) Merge this into ~/.claude/settings.json to show live session status in the dashboard:\n")
	sb.Write(b)
	sb.WriteString("\n")
	return sb.String()
}

func printHooksConfig() { fmt.Print(hooksConfigText()) }
