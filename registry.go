package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Extension points so feature modules live in their own files and never edit shared ones.
//
//	func init() {
//		addRoutes(func(mux *http.ServeMux) { mux.HandleFunc("GET /api/docker/ps", ...) })
//		registerMCPTool(tool{Name: "list_containers", ...})
//	}

var routeRegistrars []func(*http.ServeMux)

// addRoutes registers HTTP handlers; they are mounted behind the auth guard.
func addRoutes(f func(*http.ServeMux)) { routeRegistrars = append(routeRegistrars, f) }

var mcpExtra []tool

// registerMCPTool adds a tool to `devdeck mcp`.
func registerMCPTool(t tool) { mcpExtra = append(mcpExtra, t) }

var storeNameRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
var storeMu sync.Mutex

// loadJSON reads ~/.devdeck/<name>.json into v. A missing file leaves v untouched and returns nil.
func loadJSON(name string, v any) error {
	if !storeNameRe.MatchString(name) {
		return errors.New("bad store name")
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dataDir(), name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// saveJSON atomically writes v to ~/.devdeck/<name>.json (mode 0600). Never store secrets here.
func saveJSON(name string, v any) error {
	if !storeNameRe.MatchString(name) {
		return errors.New("bad store name")
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	p := filepath.Join(dataDir(), name+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
