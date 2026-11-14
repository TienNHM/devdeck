package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type rpcReq struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	handler     func(args map[string]any) (string, error)
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func typ(t, desc string) map[string]any { return map[string]any{"type": t, "description": desc} }

func argStr(a map[string]any, k, def string) string {
	if s, ok := a[k].(string); ok && s != "" {
		return s
	}
	return def
}

func argInt(a map[string]any, k string, def int) int {
	if f, ok := a[k].(float64); ok {
		return int(f)
	}
	return def
}

func mcpTools() []tool {
	return []tool{
		{Name: "list_ports", Description: "List TCP ports in LISTEN state with owning process name and PID.",
			InputSchema: obj(map[string]any{}),
			handler: func(map[string]any) (string, error) {
				ps, err := listPorts()
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, p := range ps {
					fmt.Fprintf(&sb, "%s:%d\tpid=%d\t%s\n", p.Addr, p.Port, p.PID, p.Process)
				}
				return sb.String(), nil
			}},
		{Name: "list_processes", Description: "List processes sorted by memory (RSS) descending.",
			InputSchema: obj(map[string]any{"limit": typ("integer", "max rows, default 40")}),
			handler: func(a map[string]any) (string, error) {
				ps, err := listProcesses(argInt(a, "limit", 40))
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, p := range ps {
					fmt.Fprintf(&sb, "pid=%d\tppid=%d\t%.1fMB\t%s\n", p.PID, p.PPID, float64(p.RSS)/1048576, p.Name)
				}
				return sb.String(), nil
			}},
		{Name: "kill_process", Description: "Kill a process by PID (optionally its whole tree). Destructive.",
			InputSchema: obj(map[string]any{"pid": typ("integer", "process id"), "tree": typ("boolean", "also kill children")}, "pid"),
			handler: func(a map[string]any) (string, error) {
				tree, _ := a["tree"].(bool)
				if err := killProcess(int32(argInt(a, "pid", 0)), tree); err != nil {
					return "", err
				}
				return "killed", nil
			}},
		{Name: "git_graph", Description: "ASCII commit graph of all branches in a repository (newest first).",
			InputSchema: obj(map[string]any{"repo": typ("string", "path to the repository"), "limit": typ("integer", "max commits, default 40")}, "repo"),
			handler: func(a map[string]any) (string, error) {
				g, err := gitGraph(argStr(a, "repo", "."), argInt(a, "limit", 40))
				if err != nil {
					return "", err
				}
				return renderASCIIGraph(g), nil
			}},
		{Name: "git_show", Description: "Show a commit with stat (add patch=true for the full diff).",
			InputSchema: obj(map[string]any{"repo": typ("string", "repository path"), "hash": typ("string", "commit hash"), "patch": typ("boolean", "include full diff")}, "repo", "hash"),
			handler: func(a map[string]any) (string, error) {
				if p, _ := a["patch"].(bool); p {
					return gitDiff(argStr(a, "repo", "."), argStr(a, "hash", ""))
				}
				return gitCommit(argStr(a, "repo", "."), argStr(a, "hash", ""))
			}},
		{Name: "redis_scan", Description: "Read-only SCAN of Redis keys with type and TTL. Password comes from env DEVDECK_REDIS_PASSWORD.",
			InputSchema: obj(map[string]any{"addr": typ("string", "host:port, default 127.0.0.1:6379"), "db": typ("integer", "database index"),
				"match": typ("string", "glob pattern, default *"), "count": typ("integer", "scan hint, default 200"), "tls": typ("boolean", "use TLS")}),
			handler: func(a map[string]any) (string, error) {
				c := newRedisClient(redisConnReq{Addr: argStr(a, "addr", "127.0.0.1:6379"), Password: os.Getenv("DEVDECK_REDIS_PASSWORD"), DB: argInt(a, "db", 0), TLS: a["tls"] == true})
				defer c.Close()
				r, err := redisScan(c, 0, argStr(a, "match", "*"), int64(argInt(a, "count", 200)))
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, k := range r.Keys {
					fmt.Fprintf(&sb, "%s\t%s\tttl=%d\n", k.Key, k.Type, k.TTL)
				}
				fmt.Fprintf(&sb, "next_cursor=%d\n", r.Cursor)
				return sb.String(), nil
			}},
		{Name: "redis_get", Description: "Read-only fetch of one Redis key (any type, size-limited).",
			InputSchema: obj(map[string]any{"addr": typ("string", "host:port"), "db": typ("integer", "database index"), "key": typ("string", "key name"), "tls": typ("boolean", "use TLS")}, "key"),
			handler: func(a map[string]any) (string, error) {
				c := newRedisClient(redisConnReq{Addr: argStr(a, "addr", "127.0.0.1:6379"), Password: os.Getenv("DEVDECK_REDIS_PASSWORD"), DB: argInt(a, "db", 0), TLS: a["tls"] == true})
				defer c.Close()
				kv, err := redisGet(c, argStr(a, "key", ""))
				if err != nil {
					return "", err
				}
				b, _ := json.MarshalIndent(kv, "", "  ")
				return string(b), nil
			}},
		{Name: "ssh_exec", Description: "Run one command on a SSH host saved in ~/.devdeck/config.json (key auth only). Output capped at 64KB.",
			InputSchema: obj(map[string]any{"host": typ("string", "saved host name"), "command": typ("string", "command line"), "timeout_sec": typ("integer", "default 30")}, "host", "command"),
			handler: func(a map[string]any) (string, error) {
				return sshExec(argStr(a, "host", ""), argStr(a, "command", ""), time.Duration(argInt(a, "timeout_sec", 30))*time.Second)
			}},
		{Name: "claude_sessions", Description: "Live Claude Code sessions reported through devdeck hooks (needs the dashboard running).",
			InputSchema: obj(map[string]any{}),
			handler: func(map[string]any) (string, error) {
				port := readPortFile()
				if port == 0 {
					return "", fmt.Errorf("devdeck dashboard is not running")
				}
				req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/claude/sessions", port), nil)
				req.Header.Set("Authorization", "Bearer "+loadToken())
				resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				return string(b), nil
			}},
	}
}

func renderASCIIGraph(g *GraphResult) string {
	var sb strings.Builder
	for _, r := range g.Rows {
		w := g.Lanes
		line := make([]byte, w*2)
		for i := range line {
			line[i] = ' '
		}
		for _, l := range r.Pass {
			line[l*2] = '|'
		}
		for _, l := range r.Conv {
			if l != r.Col {
				line[l*2] = '/'
			}
		}
		line[r.Col*2] = '*'
		refs := ""
		if len(r.Refs) > 0 {
			refs = " (" + strings.Join(r.Refs, ", ") + ")"
		}
		fmt.Fprintf(&sb, "%s %s%s %s [%s, %s]\n", strings.TrimRight(string(line), " "), r.Hash[:7], refs, r.Subject, r.Author, time.Unix(r.Time, 0).Format("2006-01-02"))
	}
	if g.Truncate {
		sb.WriteString("... more commits not shown\n")
	}
	return sb.String()
}

func runMCP() {
	tools := mcpTools()
	byName := map[string]tool{}
	for _, t := range tools {
		byName[t.Name] = t
	}
	var mu sync.Mutex
	enc := json.NewEncoder(os.Stdout)
	send := func(id json.RawMessage, result any, rerr map[string]any) {
		m := map[string]any{"jsonrpc": "2.0", "id": id}
		if rerr != nil {
			m["error"] = rerr
		} else {
			m["result"] = result
		}
		mu.Lock()
		_ = enc.Encode(m)
		mu.Unlock()
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		var req rpcReq
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // notifications and garbage need no reply
		}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2024-11-05"
			}
			send(req.ID, map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "devdeck", "version": version},
			}, nil)
		case "ping":
			send(req.ID, map[string]any{}, nil)
		case "tools/list":
			send(req.ID, map[string]any{"tools": tools}, nil)
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			t, ok := byName[p.Name]
			if !ok {
				send(req.ID, nil, map[string]any{"code": -32602, "message": "unknown tool: " + p.Name})
				continue
			}
			text, err := func() (s string, err error) {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				return t.handler(p.Arguments)
			}()
			isErr := err != nil
			if isErr {
				text = err.Error()
			}
			if len(text) > 100*1024 {
				text = text[:100*1024] + "\n... truncated ..."
			}
			send(req.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": isErr}, nil)
		default:
			send(req.ID, nil, map[string]any{"code": -32601, "message": "method not found"})
		}
	}
}
