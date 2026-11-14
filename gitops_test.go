package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive the real git binary against throwaway repositories in t.TempDir().

func tg(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, file, content, msg string) string {
	write(t, dir, file, content)
	tg(t, dir, "add", "-A")
	tg(t, dir, "commit", "-q", "-m", msg)
	return tg(t, dir, "rev-parse", "HEAD")
}

func act(t *testing.T, repo, action string, a aargs) string {
	t.Helper()
	out, err := execGitAction(repo, action, a)
	if err != nil {
		t.Fatalf("%s failed: %v", action, err)
	}
	return out
}

func actFails(t *testing.T, repo, action string, a aargs) string {
	t.Helper()
	_, err := execGitAction(repo, action, a)
	if err == nil {
		t.Fatalf("%s should have failed", action)
	}
	return err.Error()
}

func setupEnv(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Tester")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Tester")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	empty := filepath.Join(t.TempDir(), "gitconfig")
	_ = os.WriteFile(empty, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
}

func newRepo(t *testing.T) string {
	setupEnv(t)
	dir := t.TempDir()
	tg(t, dir, "init", "-q", "-b", "main")
	commit(t, dir, "a.txt", "one\n", "first")
	commit(t, dir, "a.txt", "one\ntwo\n", "second")
	return dir
}

func TestGraphMergeStashUncommitted(t *testing.T) {
	repo := newRepo(t)
	base := tg(t, repo, "rev-parse", "HEAD")
	act(t, repo, "branch.create", aargs{"name": "feat", "hash": base, "checkout": true})
	commit(t, repo, "f.txt", "f\n", "feat work :sparkles:")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	commit(t, repo, "m.txt", "m\n", "main work")
	act(t, repo, "branch.merge", aargs{"target": "feat", "noFF": true})

	g, err := gitGraphOpts(repo, GraphOpts{Limit: 50, ShowRemote: true, ShowTags: true, ShowStashes: true, ShowUncommitted: true, InHead: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rows) != 5 || g.Branch != "main" {
		t.Fatalf("rows=%d branch=%q", len(g.Rows), g.Branch)
	}
	merge := g.Rows[0]
	if len(merge.Parents) != 2 || len(merge.Out) != 2 {
		t.Fatalf("merge row should have 2 parents/out lanes: %+v", merge)
	}
	if g.Lanes < 2 {
		t.Fatalf("expected at least 2 lanes, got %d", g.Lanes)
	}
	if !merge.RefInfo[0].Head || merge.RefInfo[0].Name != "main" {
		t.Fatalf("HEAD ref missing: %+v", merge.RefInfo)
	}

	// uncommitted changes row
	write(t, repo, "a.txt", "changed\n")
	write(t, repo, "new.txt", "untracked\n")
	g, _ = gitGraphOpts(repo, GraphOpts{Limit: 50, ShowRemote: true, ShowTags: true, ShowUncommitted: true})
	if g.Rows[0].Hash != "*" || g.Rows[0].Changes != 2 {
		t.Fatalf("uncommitted row wrong: %+v", g.Rows[0])
	}
	files, err := diffFiles(repo, g.Head, "WORKTREE")
	if err != nil || len(files) != 2 {
		t.Fatalf("worktree files: %v %+v", err, files)
	}
	st := map[string]string{}
	for _, f := range files {
		st[f.Path] = f.Status
	}
	if st["a.txt"] != "M" || st["new.txt"] != "U" {
		t.Fatalf("statuses: %+v", st)
	}
	d, err := fileDiff(repo, g.Head, "WORKTREE", "new.txt", "", true)
	if err != nil || !strings.Contains(d, "+untracked") {
		t.Fatalf("untracked diff: %v %q", err, d)
	}

	// stash push (+untracked) -> stash row, then apply/branch/drop
	act(t, repo, "stash.push", aargs{"message": "wip", "includeUntracked": true})
	g, _ = gitGraphOpts(repo, GraphOpts{Limit: 50, ShowRemote: true, ShowTags: true, ShowStashes: true, ShowUncommitted: true})
	var stashRows int
	for _, r := range g.Rows {
		if r.Stash != "" {
			stashRows++
			if len(r.Parents) != 1 {
				t.Fatalf("stash row should keep only its base parent: %+v", r)
			}
		}
	}
	if stashRows != 1 || g.Rows[0].Hash == "*" {
		t.Fatalf("stash rows=%d first=%q (tree should be clean)", stashRows, g.Rows[0].Hash)
	}
	if len(g.Rows) != 6 {
		t.Fatalf("stash helper commits must be hidden, rows=%d", len(g.Rows))
	}
	act(t, repo, "stash.apply", aargs{"selector": "stash@{0}"})
	act(t, repo, "uncommitted.reset", aargs{"mode": "hard"})
	act(t, repo, "uncommitted.clean", aargs{"directories": true})
	act(t, repo, "stash.branch", aargs{"selector": "stash@{0}", "name": "from-stash"})
	if tg(t, repo, "rev-parse", "--abbrev-ref", "HEAD") != "from-stash" {
		t.Fatal("stash.branch should check out the new branch")
	}
	act(t, repo, "uncommitted.reset", aargs{"mode": "hard"})
	act(t, repo, "uncommitted.clean", aargs{"directories": true})
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	act(t, repo, "branch.delete", aargs{"name": "from-stash", "force": true})
}

func TestCommitActions(t *testing.T) {
	repo := newRepo(t)
	head := tg(t, repo, "rev-parse", "HEAD")

	// tags
	act(t, repo, "tag.add", aargs{"name": "v1", "hash": head, "message": "release one"})
	act(t, repo, "tag.add", aargs{"name": "light", "hash": head, "lightweight": true})
	td, err := tagDetails(repo, "v1")
	if err != nil || !td.Annotated || td.Tagger != "Tester" || td.Message != "release one" {
		t.Fatalf("tag details: %v %+v", err, td)
	}
	if td, _ := tagDetails(repo, "light"); td.Annotated {
		t.Fatal("lightweight tag must not be annotated")
	}
	if !strings.Contains(actFails(t, repo, "tag.add", aargs{"name": "v2", "hash": head}), "message") {
		t.Fatal("annotated tag without message must fail")
	}
	act(t, repo, "tag.delete", aargs{"name": "light"})

	// cherry-pick / revert
	act(t, repo, "branch.create", aargs{"name": "side", "hash": head, "checkout": true})
	pick := commit(t, repo, "side.txt", "s\n", "side commit")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	act(t, repo, "commit.cherrypick", aargs{"hash": pick, "record": true})
	if !strings.Contains(tg(t, repo, "log", "-1", "--format=%B"), "cherry picked from commit") {
		t.Fatal("-x should record the origin")
	}
	act(t, repo, "commit.revert", aargs{"hash": "HEAD"})
	if _, err := os.Stat(filepath.Join(repo, "side.txt")); err == nil {
		t.Fatal("revert should have removed side.txt")
	}

	// reset modes
	before := tg(t, repo, "rev-parse", "HEAD")
	act(t, repo, "reset", aargs{"to": head, "mode": "soft"})
	if tg(t, repo, "rev-parse", "HEAD") != head {
		t.Fatal("reset --soft did not move HEAD")
	}
	act(t, repo, "reset", aargs{"to": before, "mode": "hard"})
	if tg(t, repo, "rev-parse", "HEAD") != before {
		t.Fatal("reset --hard did not move HEAD")
	}

	// drop
	extra := commit(t, repo, "x.txt", "x\n", "to be dropped")
	commit(t, repo, "y.txt", "y\n", "keep me")
	act(t, repo, "commit.drop", aargs{"hash": extra})
	if strings.Contains(tg(t, repo, "log", "--format=%s"), "to be dropped") {
		t.Fatal("commit was not dropped")
	}
	if _, err := os.Stat(filepath.Join(repo, "y.txt")); err != nil {
		t.Fatal("later commit must survive the drop")
	}

	// detached checkout, contains, details
	act(t, repo, "commit.checkout", aargs{"hash": head})
	if tg(t, repo, "rev-parse", "--abbrev-ref", "HEAD") != "HEAD" {
		t.Fatal("expected detached HEAD")
	}
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	c, err := commitContains(repo, head)
	if err != nil || !c.InHead || len(c.Branches) < 2 || len(c.Tags) != 1 {
		t.Fatalf("contains: %v %+v", err, c)
	}
	det, err := commitDetails(repo, pick)
	if err != nil || det.Subject != "side commit" || len(det.Files) != 1 || det.Files[0].Status != "A" || det.Files[0].Add != 1 {
		t.Fatalf("details: %v %+v", err, det)
	}
	fc, err := fileAt(repo, pick, "side.txt")
	if err != nil || fc.Text != "s\n" {
		t.Fatalf("fileAt: %v %+v", err, fc)
	}
	d, err := fileDiff(repo, det.Parents[0], pick, "side.txt", "", false)
	if err != nil || !strings.Contains(d, "+s") {
		t.Fatalf("fileDiff: %v %q", err, d)
	}
	cmp, err := diffFiles(repo, head, pick)
	if err != nil || len(cmp) != 1 {
		t.Fatalf("compare: %v %+v", err, cmp)
	}
}

func TestRemotesPushFetchDelete(t *testing.T) {
	repo := newRepo(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	tg(t, repo, "init", "-q", "--bare", bare)
	act(t, repo, "remote.add", aargs{"name": "origin", "url": bare})
	act(t, repo, "branch.push", aargs{"branch": "main", "remotes": []any{"origin"}, "setUpstream": true})
	act(t, repo, "branch.create", aargs{"name": "topic", "hash": "main", "checkout": true})
	commit(t, repo, "t.txt", "t\n", "topic")
	act(t, repo, "branch.push", aargs{"branch": "topic", "remotes": []any{"origin"}, "setUpstream": true})
	act(t, repo, "tag.add", aargs{"name": "rel", "hash": "topic", "message": "m", "pushRemote": "origin"})
	if !strings.Contains(tg(t, bare, "tag", "-l"), "rel") {
		t.Fatal("tag was not pushed")
	}

	refs, err := listRefs(repo)
	if err != nil || len(refs.Remotes) != 1 || refs.Remotes[0].Fetch != bare || refs.Head != "topic" {
		t.Fatalf("refs: %v %+v", err, refs)
	}
	var up string
	for _, b := range refs.Branches {
		if b.Name == "topic" {
			up = b.Upstream
		}
	}
	if up != "origin/topic" {
		t.Fatalf("upstream = %q", up)
	}

	// someone else pushes a commit; fetch then pull it
	other := t.TempDir()
	tg(t, other, "clone", "-q", bare, "o")
	oc := filepath.Join(other, "o")
	tg(t, oc, "checkout", "-q", "topic")
	commit(t, oc, "o.txt", "o\n", "from other")
	tg(t, oc, "push", "-q", "origin", "topic")

	act(t, repo, "remote.fetch", aargs{"prune": true})
	act(t, repo, "branch.pull", aargs{"remote": "origin", "branch": "topic"})
	if !strings.Contains(tg(t, repo, "log", "--format=%s"), "from other") {
		t.Fatal("pull did not bring the commit")
	}
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	act(t, repo, "branch.fetch_into", aargs{"remote": "origin", "remoteBranch": "topic", "name": "topic2"})
	act(t, repo, "branch.checkout_remote", aargs{"remoteBranch": "origin/topic", "name": "tracking"})
	act(t, repo, "branch.checkout", aargs{"name": "main"})

	// force push rejected without force, accepted with force-with-lease
	act(t, repo, "branch.checkout", aargs{"name": "topic"})
	act(t, repo, "reset", aargs{"to": tg(t, repo, "rev-parse", "HEAD~1"), "mode": "hard"})
	commit(t, repo, "diverge.txt", "d\n", "diverge")
	actFails(t, repo, "branch.push", aargs{"branch": "topic", "remotes": []any{"origin"}})
	act(t, repo, "branch.push", aargs{"branch": "topic", "remotes": []any{"origin"}, "mode": "force-with-lease"})
	act(t, repo, "branch.checkout", aargs{"name": "main"})

	// remote edit / delete branches, prune, delete remote
	act(t, repo, "branch.delete", aargs{"name": "topic", "force": true, "remote": "origin"})
	if strings.Contains(tg(t, bare, "branch", "--list"), "topic") {
		t.Fatal("remote branch should be gone")
	}
	act(t, repo, "branch.delete_remote", aargs{"remote": "origin", "name": "main"})
	act(t, repo, "remote.edit", aargs{"old": "origin", "name": "upstream", "url": bare, "pushUrl": bare})
	act(t, repo, "remote.prune", aargs{"name": "upstream"})
	act(t, repo, "remote.delete", aargs{"name": "upstream"})
	if r, _ := listRefs(repo); len(r.Remotes) != 0 {
		t.Fatalf("remote still present: %+v", r.Remotes)
	}
}

func TestRebaseConflictsAndOps(t *testing.T) {
	repo := newRepo(t)
	base := tg(t, repo, "rev-parse", "HEAD")
	act(t, repo, "branch.create", aargs{"name": "topic", "hash": base, "checkout": true})
	commit(t, repo, "c.txt", "topic\n", "topic change")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	commit(t, repo, "m.txt", "main\n", "main change")
	act(t, repo, "branch.checkout", aargs{"name": "topic"})
	act(t, repo, "branch.rebase", aargs{"onto": "main"})
	if tg(t, repo, "rev-list", "--count", "main..topic") != "1" {
		t.Fatal("rebase should leave exactly one commit on topic")
	}

	// conflicting merge -> error, state=merge, abort
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	act(t, repo, "branch.create", aargs{"name": "left", "hash": "main", "checkout": true})
	commit(t, repo, "a.txt", "left\n", "left edit")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	commit(t, repo, "a.txt", "right\n", "right edit")
	actFails(t, repo, "branch.merge", aargs{"target": "left"})
	refs, _ := listRefs(repo)
	if refs.State != "merge" {
		t.Fatalf("expected merge in progress, got %q", refs.State)
	}
	files, _ := diffFiles(repo, refs.HeadHash, "WORKTREE")
	if len(files) == 0 || files[0].Status != "!" {
		t.Fatalf("conflicted file should be flagged: %+v", files)
	}
	act(t, repo, "op.abort", aargs{"op": "merge"})
	if refs, _ = listRefs(repo); refs.State != "" {
		t.Fatalf("state after abort: %q", refs.State)
	}

	// squash merge commits automatically
	act(t, repo, "branch.create", aargs{"name": "sq", "hash": "main", "checkout": true})
	commit(t, repo, "sq.txt", "q\n", "sq work")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	act(t, repo, "branch.merge", aargs{"target": "sq", "squash": true})
	if _, err := os.Stat(filepath.Join(repo, "sq.txt")); err != nil {
		t.Fatal("squash merge should bring the file")
	}
	act(t, repo, "branch.rename", aargs{"old": "sq", "name": "sq2"})
}

func TestValidationRejectsInjection(t *testing.T) {
	repo := newRepo(t)
	bad := []struct {
		action string
		args   aargs
	}{
		{"branch.create", aargs{"name": "-D", "hash": "main"}},
		{"branch.create", aargs{"name": "a..b", "hash": "main"}},
		{"branch.checkout", aargs{"name": "--orphan=x"}},
		{"branch.checkout", aargs{"name": "x y"}},
		{"branch.merge", aargs{"target": "main; rm -rf /"}},
		{"reset", aargs{"to": "HEAD", "mode": "--hard"}},
		{"reset", aargs{"to": "-q", "mode": "hard"}},
		{"remote.add", aargs{"name": "x", "url": "ext::sh -c touch /tmp/pwn"}},
		{"remote.add", aargs{"name": "x", "url": "--upload-pack=evil"}},
		{"remote.add", aargs{"name": "-x", "url": "https://example.com/r.git"}},
		{"stash.apply", aargs{"selector": "stash@{0}; id"}},
		{"stash.drop", aargs{"selector": "HEAD"}},
		{"commit.cherrypick", aargs{"hash": "main", "parent": 99}},
		{"branch.push", aargs{"branch": "main", "remotes": []any{"--mirror"}}},
		{"branch.push", aargs{"branch": "main", "remotes": []any{"origin"}, "mode": "--delete"}},
		{"tag.delete", aargs{"name": "../x"}},
		{"nope.action", aargs{}},
	}
	for _, c := range bad {
		if _, err := execGitAction(repo, c.action, c.args); err == nil {
			t.Errorf("%s %v should be rejected", c.action, c.args)
		}
	}
	if _, err := cleanRepo("-C"); err == nil {
		t.Error("repo path starting with '-' must be rejected")
	}
	for _, p := range []string{"../x", "/etc/passwd", "a/../../b", `C:\x`, ""} {
		if validRelPath(p) {
			t.Errorf("path %q should be invalid", p)
		}
	}
	if _, err := fileAt(repo, "main", "../../secret"); err == nil {
		t.Error("fileAt must reject traversal")
	}
	if _, err := gitGraphOpts(repo, GraphOpts{Selectors: []string{"--all"}}); err == nil {
		t.Error("graph selector --all must be rejected")
	}
	if _, err := gitGraphOpts(repo, GraphOpts{Selectors: []string{"glob:--output=x"}}); err == nil {
		t.Error("graph glob starting with - must be rejected")
	}
}

func TestSelectorsFirstParentAndLegacy(t *testing.T) {
	repo := newRepo(t)
	base := tg(t, repo, "rev-parse", "HEAD")
	act(t, repo, "branch.create", aargs{"name": "feature/x", "hash": base, "checkout": true})
	commit(t, repo, "x.txt", "x\n", "on feature")
	act(t, repo, "branch.checkout", aargs{"name": "main"})
	commit(t, repo, "m.txt", "m\n", "on main")
	act(t, repo, "branch.merge", aargs{"target": "feature/x", "noFF": true})

	g, err := gitGraphOpts(repo, GraphOpts{Selectors: []string{"refs/heads/feature/x"}, ShowTags: true})
	if err != nil || len(g.Rows) != 3 {
		t.Fatalf("single-branch filter rows=%d err=%v", len(g.Rows), err)
	}
	g, err = gitGraphOpts(repo, GraphOpts{Selectors: []string{"glob:heads/feature/*"}})
	if err != nil || len(g.Rows) != 3 {
		t.Fatalf("glob filter rows=%d err=%v", len(g.Rows), err)
	}
	g, _ = gitGraphOpts(repo, GraphOpts{ShowRemote: true, FirstParent: true, Order: "topo"})
	for _, r := range g.Rows {
		if len(r.Parents) > 1 {
			t.Fatal("first-parent mode must keep one parent per commit")
		}
	}
	// the legacy entry points used by MCP and the old route
	lg, err := gitGraph(repo, 10)
	if err != nil || len(lg.Rows) == 0 || renderASCIIGraph(lg) == "" {
		t.Fatalf("legacy graph: %v", err)
	}
	if _, err := gitCommit(repo, base); err != nil {
		t.Fatal(err)
	}
	if _, err := gitDiff(repo, base); err != nil {
		t.Fatal(err)
	}
	sig1, _ := repoSignature(repo)
	write(t, repo, "zzz.txt", "z")
	sig2, _ := repoSignature(repo)
	if sig1 == "" || sig1 == sig2 {
		t.Fatal("signature must change when the work tree changes")
	}
	found := discoverRepos(filepath.Dir(repo), 1, 10)
	if len(found) == 0 {
		t.Fatal("discoverRepos found nothing")
	}
}

func TestEmptyRepo(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	tg(t, dir, "init", "-q", "-b", "main")
	g, err := gitGraphOpts(dir, GraphOpts{ShowRemote: true, ShowTags: true, ShowStashes: true, ShowUncommitted: true})
	if err != nil || len(g.Rows) != 0 {
		t.Fatalf("empty repo: %v %+v", err, g)
	}
	if _, err := listRefs(dir); err != nil {
		t.Fatal(err)
	}
}
