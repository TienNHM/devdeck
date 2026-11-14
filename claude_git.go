package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Read-only git context collected for "Ask Claude" prompts (explain commit, commit message, PR description).

const gitCtxMax = 60 << 10

func capText(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	return strings.ToValidUTF8(s[:n], "") + "\n... (diff truncated)\n", true
}

func gitRead(repo string, max int, args ...string) (string, error) {
	out, se, err := runGit(repo, 30*time.Second, max, nil, args...)
	if err != nil {
		return "", gitErr(se, err, out)
	}
	return b2s(out), nil
}

// claudeGitContext returns text for one of: commit (hash), uncommitted (staged / all), branch (base..branch).
func claudeGitContext(repoIn, kind string, q map[string]string) (string, bool, error) {
	repo, err := cleanRepo(repoIn)
	if err != nil {
		return "", false, err
	}
	diffFlags := []string{"--no-color", "--no-ext-diff", "--no-textconv"}
	var sb strings.Builder
	switch kind {
	case "commit":
		h := q["hash"]
		if !hashRe.MatchString(h) {
			return "", false, errors.New("invalid commit hash")
		}
		stat, err := gitRead(repo, 64<<10, append([]string{"show", "--stat", "--format=fuller"}, append(diffFlags, h)...)...)
		if err != nil {
			return "", false, err
		}
		patch, err := gitRead(repo, gitCtxMax+1024, append([]string{"show", "--format=", "--patch"}, append(diffFlags, h)...)...)
		if err != nil {
			return "", false, err
		}
		sb.WriteString(stat + "\n")
		p, cut := capText(patch, gitCtxMax)
		sb.WriteString(p)
		return sb.String(), cut, nil
	case "uncommitted":
		st, _ := gitRead(repo, 32<<10, "status", "--short", "--branch")
		sb.WriteString("git status:\n" + st + "\n")
		hasHead := revParse(repo, "HEAD") != ""
		var args []string
		if q["staged"] == "1" || !hasHead {
			args = append([]string{"diff", "--cached"}, diffFlags...)
		} else {
			args = append([]string{"diff", "HEAD"}, diffFlags...)
		}
		stat, err := gitRead(repo, 32<<10, append(append([]string{}, args...), "--stat")...)
		if err != nil {
			return "", false, err
		}
		patch, err := gitRead(repo, gitCtxMax+1024, args...)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(patch) == "" && q["staged"] == "1" {
			return "", false, errors.New("nothing is staged")
		}
		sb.WriteString(stat + "\n")
		p, cut := capText(patch, gitCtxMax)
		sb.WriteString(p)
		return sb.String(), cut, nil
	case "branch":
		br, base := q["branch"], q["base"]
		if !validRev(br) || !validRev(base) {
			return "", false, errors.New("invalid branch name")
		}
		mb, err := gitRead(repo, 256, "merge-base", base, br)
		if err != nil {
			return "", false, fmt.Errorf("no common ancestor of %s and %s", base, br)
		}
		rng := strings.TrimSpace(mb) + ".." + br
		log, err := gitRead(repo, 32<<10, "log", "--no-color", "--format=- %h %s%n%b", "-n", "200", rng)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(log) == "" {
			return "", false, fmt.Errorf("%s has no commits that are not in %s", br, base)
		}
		stat, _ := gitRead(repo, 32<<10, append([]string{"diff", "--stat"}, append(diffFlags, rng)...)...)
		patch, err := gitRead(repo, gitCtxMax+1024, append([]string{"diff"}, append(diffFlags, rng)...)...)
		if err != nil {
			return "", false, err
		}
		fmt.Fprintf(&sb, "Branch %s compared with %s.\n\nCommits:\n%s\nFiles:\n%s\n", br, base, log, stat)
		p, cut := capText(patch, gitCtxMax)
		sb.WriteString(p)
		return sb.String(), cut, nil
	}
	return "", false, errors.New("unknown context kind")
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/claude/gitctx", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			m := map[string]string{"hash": q.Get("hash"), "staged": q.Get("staged"), "branch": q.Get("branch"), "base": q.Get("base")}
			text, cut, err := claudeGitContext(q.Get("repo"), q.Get("kind"), m)
			if err != nil {
				fail(w, 400, err)
				return
			}
			writeJSON(w, map[string]any{"text": text, "truncated": cut})
		})
	})
}
