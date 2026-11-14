package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type ClaudeSession struct {
	ID      string `json:"id"`
	Cwd     string `json:"cwd"`
	State   string `json:"state"` // idle | working | waiting
	Event   string `json:"event"`
	Message string `json:"message,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Updated int64  `json:"updated"`
}

var claudeSessions = struct {
	sync.Mutex
	m map[string]*ClaudeSession
}{m: map[string]*ClaudeSession{}}

func recordClaudeEvent(p map[string]any) {
	str := func(k string) string { s, _ := p[k].(string); return s }
	id, event := str("session_id"), str("hook_event_name")
	if id == "" || event == "" {
		return
	}
	claudeSessions.Lock()
	defer claudeSessions.Unlock()
	if event == "SessionEnd" {
		delete(claudeSessions.m, id)
		return
	}
	s := claudeSessions.m[id]
	if s == nil {
		s = &ClaudeSession{ID: id}
		claudeSessions.m[id] = s
	}
	if cwd := str("cwd"); cwd != "" {
		s.Cwd = cwd
	}
	s.Event, s.Updated = event, time.Now().Unix()
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
	cutoff := time.Now().Add(-12 * time.Hour).Unix()
	for k, v := range claudeSessions.m {
		if v.Updated < cutoff {
			delete(claudeSessions.m, k)
		}
	}
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

func printHooksConfig() {
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
	fmt.Println("# 1) Register the MCP server with Claude Code:")
	fmt.Printf("claude mcp add devdeck -- \"%s\" mcp\n\n", exe)
	fmt.Println("# 2) Merge this into ~/.claude/settings.json to show live session status in the dashboard:")
	fmt.Println(string(b))
}
