package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ------------------------------------------------------------ process helpers

type capBuf struct {
	buf bytes.Buffer
	max int
}

func (c *capBuf) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}

func gitEnv() []string {
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true",
		"LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat")
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

// runGit runs git with an argument slice (never a shell). Output is capped at maxOut bytes.
func runGit(repo string, timeout time.Duration, maxOut int, stdin io.Reader, args ...string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	full := append([]string{"-C", repo, "-c", "core.quotepath=false", "-c", "protocol.ext.allow=never",
		"-c", "core.fsmonitor=false", "-c", "color.ui=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = gitEnv()
	cmd.Stdin = stdin
	hideWindow(cmd)
	out := &capBuf{max: maxOut}
	errb := &capBuf{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	err := cmd.Run()
	stderr := strings.TrimSpace(errb.buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return out.buf.Bytes(), stderr, fmt.Errorf("git %s timed out after %s", args[0], timeout)
	}
	return out.buf.Bytes(), stderr, err
}

func gitErr(stderr string, err error, stdout []byte) error {
	msg := stderr
	if msg == "" {
		msg = strings.TrimSpace(string(stdout))
	}
	if msg == "" {
		msg = err.Error()
	}
	return errors.New(msg)
}

// git is the simple read helper (kept for the MCP tools and legacy routes).
func git(repo string, timeout time.Duration, args ...string) ([]byte, error) {
	out, se, err := runGit(repo, timeout, 16<<20, nil, args...)
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	return out, nil
}

// gitOut runs a command and returns trimmed stdout as a string.
func gitOut(repo string, timeout time.Duration, args ...string) (string, error) {
	b, err := git(repo, timeout, args...)
	return strings.TrimSpace(string(b)), err
}

// ------------------------------------------------------------ validation

var (
	hashRe       = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)
	refRe        = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._/+@#-]*$`)
	remoteRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,60}$`)
	stashRe      = regexp.MustCompile(`^stash@\{[0-9]{1,4}\}$`)
	globRe       = regexp.MustCompile(`^[A-Za-z0-9._/*?\[\]+@#-]+$`)
	urlSchemeRe  = regexp.MustCompile(`^[A-Za-z0-9+.-]+::`)
	validOrders  = map[string]string{"date": "--date-order", "author-date": "--author-date-order", "topo": "--topo-order"}
	validModes   = map[string]bool{"soft": true, "mixed": true, "hard": true}
	emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
)

func validRef(s string) bool {
	if len(s) == 0 || len(s) > 200 || !refRe.MatchString(s) {
		return false
	}
	if strings.Contains(s, "..") || strings.Contains(s, "//") || strings.Contains(s, "@{") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") || strings.HasSuffix(s, ".lock") || s == "@" {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}

func validRev(s string) bool { return hashRe.MatchString(s) || validRef(s) }

func validRemote(s string) bool { return remoteRe.MatchString(s) }

func validRemoteURL(s string) bool {
	if s == "" || len(s) > 2000 || strings.HasPrefix(s, "-") || urlSchemeRe.MatchString(s) {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

func validRelPath(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || len(p) > 4096 {
		return false
	}
	// Same rules on every OS: no drive letters, and both separators count, so a path that is
	// absolute or escapes the repo on Windows is also rejected when devdeck runs elsewhere.
	if len(p) >= 2 && p[1] == ':' {
		return false
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return false
		}
	}
	return true
}

func cleanRepo(p string) (string, error) {
	if p == "" || strings.HasPrefix(p, "-") || strings.ContainsRune(p, 0) {
		return "", errors.New("invalid repository path")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", errors.New("repository path is not a directory: " + p)
	}
	return abs, nil
}

func repoRoot(repo string) (string, error) {
	out, se, err := runGit(repo, 8*time.Second, 4096, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", gitErr(se, err, nil)
	}
	return filepath.FromSlash(strings.TrimSpace(string(out))), nil
}

// safeWorktreePath resolves a repo-relative path and refuses anything escaping the work tree.
func safeWorktreePath(repo, rel string) (string, error) {
	if !validRelPath(rel) {
		return "", errors.New("invalid path")
	}
	root, err := repoRoot(repo)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	er, err1 := filepath.EvalSymlinks(root)
	ea, err2 := filepath.EvalSymlinks(abs)
	if err1 != nil || err2 != nil {
		return "", errors.New("file not found")
	}
	if ea != er && !strings.HasPrefix(ea, er+string(filepath.Separator)) {
		return "", errors.New("path escapes the repository")
	}
	return ea, nil
}

// ------------------------------------------------------------ refs

type BranchInfo struct {
	Name     string `json:"name"`
	Ref      string `json:"ref"`
	Hash     string `json:"hash"`
	Current  bool   `json:"current,omitempty"`
	Upstream string `json:"upstream,omitempty"`
}

type TagInfo struct {
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	Annotated bool   `json:"annotated"`
}

type RemoteInfo struct {
	Name    string `json:"name"`
	Fetch   string `json:"fetch"`
	Push    string `json:"push"`
	Default string `json:"default,omitempty"`
}

type StashInfo struct {
	Selector string `json:"selector"`
	Hash     string `json:"hash"`
	Base     string `json:"base"`
	Time     int64  `json:"time"`
	Author   string `json:"author"`
	Subject  string `json:"subject"`
}

type RefsResult struct {
	Root           string       `json:"root"`
	Head           string       `json:"head"`
	HeadHash       string       `json:"headHash"`
	State          string       `json:"state,omitempty"`
	Branches       []BranchInfo `json:"branches"`
	RemoteBranches []BranchInfo `json:"remoteBranches"`
	Tags           []TagInfo    `json:"tags"`
	Remotes        []RemoteInfo `json:"remotes"`
	Stashes        []StashInfo  `json:"stashes"`
}

func revParse(repo, rev string) string {
	out, _, err := runGit(repo, 8*time.Second, 256, nil, "rev-parse", "-q", "--verify", rev+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func loadStashes(repo string) []StashInfo {
	out, _, err := runGit(repo, 10*time.Second, 1<<20, nil, "stash", "list", "--format=%gd%x1f%H%x1f%P%x1f%at%x1f%an%x1f%s")
	if err != nil {
		return nil
	}
	var res []StashInfo
	for _, ln := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.Split(ln, "\x1f")
		if len(f) < 6 || !stashRe.MatchString(f[0]) {
			continue
		}
		s := StashInfo{Selector: f[0], Hash: f[1], Author: f[4], Subject: f[5]}
		if ps := strings.Fields(f[2]); len(ps) > 0 {
			s.Base = ps[0]
		}
		s.Time, _ = strconv.ParseInt(f[3], 10, 64)
		res = append(res, s)
	}
	return res
}

func repoState(repo string) string {
	out, _, err := runGit(repo, 8*time.Second, 4096, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	gd := filepath.FromSlash(strings.TrimSpace(string(out)))
	has := func(n string) bool { _, e := os.Stat(filepath.Join(gd, n)); return e == nil }
	switch {
	case has("rebase-merge") || has("rebase-apply"):
		return "rebase"
	case has("MERGE_HEAD"):
		return "merge"
	case has("CHERRY_PICK_HEAD"):
		return "cherry-pick"
	case has("REVERT_HEAD"):
		return "revert"
	}
	return ""
}

func listRefs(repo string) (*RefsResult, error) {
	root, err := repoRoot(repo)
	if err != nil {
		return nil, err
	}
	res := &RefsResult{Root: root, Branches: []BranchInfo{}, RemoteBranches: []BranchInfo{}, Tags: []TagInfo{}, Remotes: []RemoteInfo{}}
	out, se, err := runGit(repo, 15*time.Second, 8<<20, nil, "for-each-ref",
		"--format=%(refname)%09%(objectname)%09%(objecttype)%09%(*objectname)%09%(upstream:short)%09%(HEAD)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	for _, ln := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.Split(ln, "\t")
		if len(f) < 6 {
			continue
		}
		ref := f[0]
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			res.Branches = append(res.Branches, BranchInfo{Name: strings.TrimPrefix(ref, "refs/heads/"), Ref: ref, Hash: f[1], Current: f[5] == "*", Upstream: f[4]})
		case strings.HasPrefix(ref, "refs/remotes/"):
			if strings.HasSuffix(ref, "/HEAD") {
				continue
			}
			res.RemoteBranches = append(res.RemoteBranches, BranchInfo{Name: strings.TrimPrefix(ref, "refs/remotes/"), Ref: ref, Hash: f[1]})
		case strings.HasPrefix(ref, "refs/tags/"):
			t := TagInfo{Name: strings.TrimPrefix(ref, "refs/tags/"), Hash: f[1], Annotated: f[2] == "tag"}
			if t.Annotated && f[3] != "" {
				t.Hash = f[3]
			}
			res.Tags = append(res.Tags, t)
		}
	}
	if b, _, err := runGit(repo, 5*time.Second, 1024, nil, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		res.Head = strings.TrimSpace(string(b))
	}
	res.HeadHash = revParse(repo, "HEAD")
	res.State = repoState(repo)
	res.Stashes = loadStashes(repo)
	if res.Stashes == nil {
		res.Stashes = []StashInfo{}
	}
	if b, _, err := runGit(repo, 8*time.Second, 1<<20, nil, "remote", "-v"); err == nil {
		idx := map[string]int{}
		for _, ln := range strings.Split(string(b), "\n") {
			fs := strings.Fields(ln)
			if len(fs) < 3 {
				continue
			}
			i, ok := idx[fs[0]]
			if !ok {
				res.Remotes = append(res.Remotes, RemoteInfo{Name: fs[0]})
				i = len(res.Remotes) - 1
				idx[fs[0]] = i
			}
			if fs[2] == "(fetch)" {
				res.Remotes[i].Fetch = fs[1]
			} else if fs[2] == "(push)" {
				res.Remotes[i].Push = fs[1]
			}
		}
	}
	return res, nil
}

// ------------------------------------------------------------ commit details

type FileChange struct {
	Path   string `json:"p"`
	Old    string `json:"o,omitempty"`
	Status string `json:"s"`
	Add    int    `json:"a"`
	Del    int    `json:"d"`
	Bin    bool   `json:"b,omitempty"`
}

type CommitDetails struct {
	Hash           string       `json:"hash"`
	Parents        []string     `json:"parents"`
	Author         string       `json:"author"`
	AuthorEmail    string       `json:"authorEmail"`
	AuthorTime     int64        `json:"authorTime"`
	Committer      string       `json:"committer"`
	CommitterEmail string       `json:"committerEmail"`
	CommitTime     int64        `json:"commitTime"`
	Subject        string       `json:"subject"`
	Body           string       `json:"body"`
	Files          []FileChange `json:"files"`
}

func emptyTree(repo string) string {
	out, _, err := runGit(repo, 5*time.Second, 256, strings.NewReader(""), "hash-object", "-t", "tree", "--stdin")
	if err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	return emptyTreeSHA
}

func mapStatus(s string) string {
	switch s {
	case "T":
		return "M"
	case "U":
		return "!"
	}
	return s
}

// diffFiles lists file changes from `from` ("" = empty tree) to `to` (a revision or "WORKTREE").
func diffFiles(repo, from, to string) ([]FileChange, error) {
	if from != "" && !validRev(from) {
		return nil, errors.New("invalid revision: " + from)
	}
	if to != "WORKTREE" && !validRev(to) {
		return nil, errors.New("invalid revision: " + to)
	}
	if from == "" {
		from = emptyTree(repo)
	}
	mk := func(mode string) []string {
		a := []string{"diff", "-M", "--no-color", "--no-ext-diff", "--no-textconv", "-z", mode, from}
		if to != "WORKTREE" {
			a = append(a, to)
		}
		return append(a, "--")
	}
	ns, se, err := runGit(repo, 30*time.Second, 16<<20, nil, mk("--name-status")...)
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	nu, _, _ := runGit(repo, 30*time.Second, 16<<20, nil, mk("--numstat")...)

	type cnt struct {
		a, d int
		bin  bool
	}
	counts := map[string]cnt{}
	toks := strings.Split(string(nu), "\x00")
	for i := 0; i < len(toks); {
		t := toks[i]
		if t == "" {
			i++
			continue
		}
		parts := strings.SplitN(t, "\t", 3)
		if len(parts) < 3 {
			i++
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(toks) {
			path = toks[i+2]
			i += 3
		} else {
			i++
		}
		c := cnt{}
		if parts[0] == "-" {
			c.bin = true
		} else {
			c.a, _ = strconv.Atoi(parts[0])
			c.d, _ = strconv.Atoi(parts[1])
		}
		counts[path] = c
	}

	files := []FileChange{}
	toks = strings.Split(string(ns), "\x00")
	for i := 0; i < len(toks); {
		t := toks[i]
		if t == "" {
			i++
			continue
		}
		st := t[:1]
		var fc FileChange
		if (st == "R" || st == "C") && i+2 < len(toks) {
			fc = FileChange{Status: st, Old: toks[i+1], Path: toks[i+2]}
			i += 3
		} else if i+1 < len(toks) {
			fc = FileChange{Status: mapStatus(st), Path: toks[i+1]}
			i += 2
		} else {
			break
		}
		if c, ok := counts[fc.Path]; ok {
			fc.Add, fc.Del, fc.Bin = c.a, c.d, c.bin
		}
		files = append(files, fc)
		if len(files) >= 5000 {
			break
		}
	}
	if to == "WORKTREE" {
		if um, _, err := runGit(repo, 10*time.Second, 4<<20, nil, "ls-files", "--unmerged", "-z"); err == nil && len(um) > 0 {
			conflicted := map[string]bool{}
			for _, rec := range strings.Split(string(um), "\x00") {
				if i := strings.IndexByte(rec, '\t'); i >= 0 {
					conflicted[rec[i+1:]] = true
				}
			}
			for i := range files {
				if conflicted[files[i].Path] {
					files[i].Status = "!"
				}
			}
		}
		ut, _, _ := runGit(repo, 20*time.Second, 4<<20, nil, "ls-files", "--others", "--exclude-standard", "-z")
		n := 0
		for _, p := range strings.Split(string(ut), "\x00") {
			if p == "" {
				continue
			}
			files = append(files, FileChange{Path: p, Status: "U"})
			if n++; n >= 2000 {
				break
			}
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func commitDetails(repo, hash string) (*CommitDetails, error) {
	if !hashRe.MatchString(hash) {
		return nil, errors.New("invalid commit hash")
	}
	out, se, err := runGit(repo, 15*time.Second, 4<<20, nil, "show", "-s", "--no-color",
		"--format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%cn%x1f%ce%x1f%ct%x1f%s%x1f%b", hash)
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	f := strings.SplitN(string(out), "\x1f", 10)
	if len(f) < 10 {
		return nil, errors.New("unexpected git output")
	}
	d := &CommitDetails{Hash: f[0], Parents: strings.Fields(f[1]), Author: f[2], AuthorEmail: f[3],
		Committer: f[5], CommitterEmail: f[6], Subject: f[8], Body: strings.TrimSpace(f[9])}
	d.AuthorTime, _ = strconv.ParseInt(f[4], 10, 64)
	d.CommitTime, _ = strconv.ParseInt(f[7], 10, 64)
	from := ""
	if len(d.Parents) > 0 {
		from = d.Parents[0]
	}
	if d.Files, err = diffFiles(repo, from, d.Hash); err != nil {
		return nil, err
	}
	return d, nil
}

// fileDiff returns a unified diff for one path between two revisions (or the work tree).
func fileDiff(repo, from, to, path, oldPath string, untracked bool) (string, error) {
	if !validRelPath(path) || (oldPath != "" && !validRelPath(oldPath)) {
		return "", errors.New("invalid path")
	}
	if untracked {
		return syntheticAddedDiff(repo, path)
	}
	if from != "" && !validRev(from) {
		return "", errors.New("invalid revision")
	}
	if to != "WORKTREE" && !validRev(to) {
		return "", errors.New("invalid revision")
	}
	if from == "" {
		from = emptyTree(repo)
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--unified=3", from}
	if to != "WORKTREE" {
		args = append(args, to)
	}
	args = append(args, "--")
	if oldPath != "" && oldPath != path {
		args = append(args, oldPath)
	}
	args = append(args, path)
	out, se, err := runGit(repo, 30*time.Second, 600<<10, nil, args...)
	if err != nil {
		return "", gitErr(se, err, nil)
	}
	return string(out), nil
}

func b2s(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return strings.ToValidUTF8(string(b), "�")
}

func isBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}

func syntheticAddedDiff(repo, path string) (string, error) {
	abs, err := safeWorktreePath(repo, path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 600<<10))
	if err != nil {
		return "", err
	}
	if isBinary(b) {
		return "Binary file " + path + " (untracked)", nil
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, len(lines))
	for _, l := range lines {
		sb.WriteString("+" + l + "\n")
	}
	return sb.String(), nil
}

type FileContent struct {
	Text   string `json:"text"`
	Binary bool   `json:"binary"`
	Size   int    `json:"size"`
}

// fileAt returns a file's content at a revision, or from the work tree when rev == "WORKTREE".
func fileAt(repo, rev, path string) (*FileContent, error) {
	if !validRelPath(path) {
		return nil, errors.New("invalid path")
	}
	var b []byte
	if rev == "WORKTREE" {
		abs, err := safeWorktreePath(repo, path)
		if err != nil {
			return nil, err
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if b, err = io.ReadAll(io.LimitReader(f, 2<<20)); err != nil {
			return nil, err
		}
	} else {
		if !validRev(rev) {
			return nil, errors.New("invalid revision")
		}
		out, se, err := runGit(repo, 20*time.Second, 2<<20, nil, "show", rev+":"+path)
		if err != nil {
			return nil, gitErr(se, err, nil)
		}
		b = out
	}
	fc := &FileContent{Size: len(b)}
	if isBinary(b) {
		fc.Binary = true
		return fc, nil
	}
	fc.Text = b2s(b)
	return fc, nil
}

// ------------------------------------------------------------ hover info, tags, discovery

type ContainsResult struct {
	InHead   bool     `json:"inHead"`
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
	Stashes  []string `json:"stashes"`
}

func commitContains(repo, hash string) (*ContainsResult, error) {
	if !hashRe.MatchString(hash) {
		return nil, errors.New("invalid commit hash")
	}
	res := &ContainsResult{Branches: []string{}, Tags: []string{}, Stashes: []string{}}
	if _, _, err := runGit(repo, 10*time.Second, 256, nil, "merge-base", "--is-ancestor", hash, "HEAD"); err == nil {
		res.InHead = true
	}
	out, se, err := runGit(repo, 20*time.Second, 1<<20, nil, "for-each-ref", "--contains", hash, "--format=%(refname)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	for _, r := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		switch {
		case strings.HasPrefix(r, "refs/heads/"):
			res.Branches = append(res.Branches, strings.TrimPrefix(r, "refs/heads/"))
		case strings.HasPrefix(r, "refs/remotes/") && !strings.HasSuffix(r, "/HEAD"):
			res.Branches = append(res.Branches, strings.TrimPrefix(r, "refs/remotes/"))
		case strings.HasPrefix(r, "refs/tags/"):
			res.Tags = append(res.Tags, strings.TrimPrefix(r, "refs/tags/"))
		}
	}
	for i, s := range loadStashes(repo) {
		if i >= 20 {
			break
		}
		if _, _, err := runGit(repo, 5*time.Second, 256, nil, "merge-base", "--is-ancestor", hash, s.Hash); err == nil {
			res.Stashes = append(res.Stashes, s.Selector)
		}
	}
	return res, nil
}

type TagDetails struct {
	Name      string `json:"name"`
	Annotated bool   `json:"annotated"`
	Tagger    string `json:"tagger,omitempty"`
	Email     string `json:"email,omitempty"`
	Date      string `json:"date,omitempty"`
	Message   string `json:"message,omitempty"`
	Hash      string `json:"hash"`
}

func tagDetails(repo, name string) (*TagDetails, error) {
	if !validRef(name) {
		return nil, errors.New("invalid tag name")
	}
	out, se, err := runGit(repo, 10*time.Second, 1<<20, nil, "for-each-ref",
		"--format=%(objecttype)%1f%(taggername)%1f%(taggeremail)%1f%(taggerdate:iso-strict)%1f%(objectname)%1f%(contents)", "refs/tags/"+name)
	if err != nil {
		return nil, gitErr(se, err, nil)
	}
	f := strings.SplitN(strings.TrimRight(string(out), "\n"), "\x1f", 6)
	if len(f) < 6 {
		return nil, errors.New("tag not found")
	}
	td := &TagDetails{Name: name, Hash: f[4]}
	if f[0] == "tag" {
		td.Annotated = true
		td.Tagger, td.Email, td.Date, td.Message = f[1], strings.Trim(f[2], "<>"), f[3], strings.TrimSpace(f[5])
	}
	return td, nil
}

func discoverRepos(root string, depth, max int) []string {
	var found []string
	skip := map[string]bool{"node_modules": true, ".git": true, "vendor": true, "target": true, "dist": true, "build": true, "$RECYCLE.BIN": true, "System Volume Information": true}
	var walk func(dir string, d int)
	walk = func(dir string, d int) {
		if len(found) >= max {
			return
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			found = append(found, dir)
			return
		}
		if d >= depth {
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if e.IsDir() && !skip[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
				walk(filepath.Join(dir, e.Name()), d+1)
			}
		}
	}
	walk(root, 0)
	sort.Strings(found)
	return found
}

// repoSignature is a cheap fingerprint of refs + HEAD + working-tree status, used for auto-refresh.
func repoSignature(repo string) (string, error) {
	h := sha1.New()
	out, se, err := runGit(repo, 15*time.Second, 8<<20, nil, "for-each-ref", "--format=%(objectname) %(refname)", "refs/heads", "refs/remotes", "refs/tags", "refs/stash")
	if err != nil {
		return "", gitErr(se, err, nil)
	}
	h.Write(out)
	h.Write([]byte(revParse(repo, "HEAD")))
	if st, _, err := runGit(repo, 20*time.Second, 4<<20, nil, "status", "--porcelain=v1", "-z", "--untracked-files=normal"); err == nil {
		h.Write(st)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
