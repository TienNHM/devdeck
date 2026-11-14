package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

func gitRepoParam(r *http.Request) (string, error) {
	return cleanRepo(r.URL.Query().Get("repo"))
}

// gitJSON runs a read handler body and writes either its value or a 400 error.
func gitJSON(w http.ResponseWriter, r *http.Request, f func(repo string) (any, error)) {
	repo, err := gitRepoParam(r)
	if err != nil {
		fail(w, 400, err)
		return
	}
	v, err := f(repo)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, v)
}

func flagParam(r *http.Request, k string, def bool) bool {
	v := r.URL.Query().Get(k)
	if v == "" {
		return def
	}
	return v == "1" || v == "true"
}

// registerGitRoutes wires the Git tab backend. All routes sit behind the auth guard in server.go.
func registerGitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/git/log", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		o := GraphOpts{
			Limit:           limit,
			Selectors:       q["b"],
			ShowRemote:      flagParam(r, "remote", true),
			ShowTags:        flagParam(r, "tags", true),
			ShowStashes:     flagParam(r, "stashes", true),
			ShowUncommitted: flagParam(r, "uncommitted", true),
			FirstParent:     flagParam(r, "firstparent", false),
			InHead:          flagParam(r, "inhead", true),
			Order:           q.Get("order"),
		}
		g, err := gitGraphOpts(q.Get("repo"), o)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, g)
	})
	mux.HandleFunc("GET /api/git/refs", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) { return listRefs(repo) })
	})
	mux.HandleFunc("GET /api/git/details", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) { return commitDetails(repo, r.URL.Query().Get("hash")) })
	})
	mux.HandleFunc("GET /api/git/uncommitted", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) {
			from := r.URL.Query().Get("against")
			if from == "" {
				from = revParse(repo, "HEAD")
			}
			files, err := diffFiles(repo, from, "WORKTREE")
			return map[string]any{"files": files, "against": from}, err
		})
	})
	mux.HandleFunc("GET /api/git/compare", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) {
			q := r.URL.Query()
			files, err := diffFiles(repo, q.Get("from"), q.Get("to"))
			return map[string]any{"files": files}, err
		})
	})
	mux.HandleFunc("GET /api/git/filediff", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) {
			q := r.URL.Query()
			text, err := fileDiff(repo, q.Get("from"), q.Get("to"), q.Get("path"), q.Get("old"), q.Get("untracked") == "1")
			return map[string]string{"text": text}, err
		})
	})
	mux.HandleFunc("GET /api/git/fileat", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) {
			q := r.URL.Query()
			return fileAt(repo, q.Get("rev"), q.Get("path"))
		})
	})
	mux.HandleFunc("GET /api/git/contains", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) { return commitContains(repo, r.URL.Query().Get("hash")) })
	})
	mux.HandleFunc("GET /api/git/tag", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) { return tagDetails(repo, r.URL.Query().Get("name")) })
	})
	mux.HandleFunc("GET /api/git/signature", func(w http.ResponseWriter, r *http.Request) {
		gitJSON(w, r, func(repo string) (any, error) {
			s, err := repoSignature(repo)
			return map[string]string{"sig": s}, err
		})
	})
	mux.HandleFunc("GET /api/git/repo", func(w http.ResponseWriter, r *http.Request) {
		p, err := cleanRepo(r.URL.Query().Get("path"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		root, err := repoRoot(p)
		if err != nil {
			fail(w, 400, errors.New("not a git repository: "+p))
			return
		}
		writeJSON(w, map[string]string{"root": root})
	})
	mux.HandleFunc("GET /api/git/discover", func(w http.ResponseWriter, r *http.Request) {
		p, err := cleanRepo(r.URL.Query().Get("path"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
		if depth <= 0 || depth > 4 {
			depth = 2
		}
		writeJSON(w, map[string]any{"repos": discoverRepos(p, depth, 200)})
	})
	mux.HandleFunc("POST /api/git/action", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Repo   string `json:"repo"`
			Action string `json:"action"`
			Args   aargs  `json:"args"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&b); err != nil {
			fail(w, 400, err)
			return
		}
		out, err := execGitAction(b.Repo, b.Action, b.Args)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "output": out})
			return
		}
		writeJSON(w, map[string]string{"output": out})
	})
}
