package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

type GRef struct {
	Name string `json:"n"`
	Type string `json:"t"` // head | remote | tag | stash | detached
	Head bool   `json:"h,omitempty"`
}

type GraphRow struct {
	Hash    string   `json:"h"`
	Parents []string `json:"p"`
	Author  string   `json:"a"`
	Email   string   `json:"e,omitempty"`
	Time    int64    `json:"at"` // author time
	CTime   int64    `json:"ct"` // committer time
	Refs    []string `json:"-"`  // plain names, used by the MCP ASCII graph
	RefInfo []GRef   `json:"r,omitempty"`
	Subject string   `json:"s"`
	Col     int      `json:"col"`
	Pass    []int    `json:"pass,omitempty"` // lanes passing straight through this row
	Conv    []int    `json:"conv,omitempty"` // lanes converging into this commit from above
	Out     []int    `json:"out,omitempty"`  // lanes the parents occupy below this commit
	InHead  bool     `json:"ih,omitempty"`
	Stash   string   `json:"st,omitempty"`
	Changes int      `json:"un,omitempty"` // Hash == "*": number of uncommitted changes
}

type GraphResult struct {
	Rows     []GraphRow `json:"rows"`
	Lanes    int        `json:"lanes"`
	Branch   string     `json:"branch"`
	Head     string     `json:"head"`
	Dirty    bool       `json:"dirty"`
	Truncate bool       `json:"truncated"`
}

type GraphOpts struct {
	Limit           int
	Selectors       []string // full ref names (refs/heads/x) or "glob:<pattern>"; empty = all
	ShowRemote      bool
	ShowTags        bool
	ShowStashes     bool
	ShowUncommitted bool
	FirstParent     bool
	Order           string
	InHead          bool
}

// gitGraph is the simple entry point used by the MCP tools and the legacy route.
func gitGraph(repo string, limit int) (*GraphResult, error) {
	return gitGraphOpts(repo, GraphOpts{Limit: limit, ShowRemote: true, ShowTags: true})
}

func countChanges(repo string) int {
	out, _, err := runGit(repo, 20*time.Second, 4<<20, nil, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return 0
	}
	n := 0
	toks := strings.Split(string(out), "\x00")
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if len(t) < 3 {
			continue
		}
		n++
		if t[0] == 'R' || t[0] == 'C' || t[1] == 'R' || t[1] == 'C' {
			i++ // rename/copy records carry the old path as an extra token
		}
	}
	return n
}

func parseDecorations(d string, o GraphOpts) (refs []GRef, plain []string) {
	if d == "" {
		return
	}
	for _, it := range strings.Split(d, ", ") {
		it = strings.TrimSpace(it)
		switch {
		case it == "HEAD":
			refs = append(refs, GRef{Name: "HEAD", Type: "detached", Head: true})
			plain = append(plain, "HEAD")
		case strings.HasPrefix(it, "HEAD -> refs/heads/"):
			n := strings.TrimPrefix(it, "HEAD -> refs/heads/")
			refs = append(refs, GRef{Name: n, Type: "head", Head: true})
			plain = append(plain, "HEAD -> "+n)
		case strings.HasPrefix(it, "refs/heads/"):
			n := strings.TrimPrefix(it, "refs/heads/")
			refs = append(refs, GRef{Name: n, Type: "head"})
			plain = append(plain, n)
		case strings.HasPrefix(it, "refs/remotes/"):
			if !o.ShowRemote || strings.HasSuffix(it, "/HEAD") {
				continue
			}
			n := strings.TrimPrefix(it, "refs/remotes/")
			refs = append(refs, GRef{Name: n, Type: "remote"})
			plain = append(plain, n)
		case strings.HasPrefix(it, "tag: refs/tags/"):
			if !o.ShowTags {
				continue
			}
			n := strings.TrimPrefix(it, "tag: refs/tags/")
			refs = append(refs, GRef{Name: n, Type: "tag"})
			plain = append(plain, "tag: "+n)
		}
	}
	return
}

func gitGraphOpts(repoIn string, o GraphOpts) (*GraphResult, error) {
	repo, err := cleanRepo(repoIn)
	if err != nil {
		return nil, err
	}
	if o.Limit <= 0 || o.Limit > 20000 {
		o.Limit = 300
	}
	flag, ok := validOrders[o.Order]
	if !ok {
		flag = validOrders["date"]
	}
	headHash := revParse(repo, "HEAD")

	var revs []string
	if len(o.Selectors) == 0 {
		revs = append(revs, "--branches")
		if o.ShowRemote {
			revs = append(revs, "--remotes")
		}
		if o.ShowTags {
			revs = append(revs, "--tags")
		}
		if headHash != "" {
			revs = append(revs, "HEAD")
		}
	} else {
		for _, s := range o.Selectors {
			switch {
			case strings.HasPrefix(s, "glob:"):
				g := strings.TrimPrefix(s, "glob:")
				if !globRe.MatchString(g) || strings.HasPrefix(g, "-") || strings.Contains(g, "..") {
					return nil, errors.New("invalid branch glob: " + g)
				}
				revs = append(revs, "--glob="+g)
			case strings.HasPrefix(s, "refs/") && validRef(s):
				revs = append(revs, s)
			default:
				return nil, errors.New("invalid branch selector: " + s)
			}
		}
	}

	stashByHash := map[string]string{}
	hide := map[string]bool{}
	if o.ShowStashes {
		for _, st := range loadStashes(repo) {
			stashByHash[st.Hash] = st.Selector
			revs = append(revs, st.Hash)
		}
	}

	res := &GraphResult{Rows: []GraphRow{}, Head: headHash}
	if b, _, err := runGit(repo, 5*time.Second, 1024, nil, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		res.Branch = strings.TrimSpace(string(b))
	}
	changes := countChanges(repo)
	res.Dirty = changes > 0

	if len(revs) == 0 {
		return res, nil
	}
	args := []string{"log", "--decorate=full", "--no-color", flag, "-n", strconv.Itoa(o.Limit + 1),
		"--format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%ct%x1f%D%x1f%s"}
	if o.FirstParent {
		args = append(args, "--first-parent")
	}
	args = append(args, revs...)
	args = append(args, "--")
	out, se, err := runGit(repo, 60*time.Second, 64<<20, nil, args...)
	if err != nil {
		if strings.Contains(se, "does not have any commits") || strings.Contains(se, "bad default revision") {
			return res, nil
		}
		return nil, gitErr(se, err, nil)
	}

	var rows []GraphRow
	for _, ln := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.SplitN(ln, "\x1f", 8)
		if len(f) < 8 {
			continue
		}
		r := GraphRow{Hash: f[0], Author: f[2], Email: f[3], Subject: f[7]}
		if f[1] != "" {
			r.Parents = strings.Fields(f[1])
		}
		r.Time, _ = strconv.ParseInt(f[4], 10, 64)
		r.CTime, _ = strconv.ParseInt(f[5], 10, 64)
		if sel, isStash := stashByHash[r.Hash]; isStash {
			r.Stash = sel
			r.RefInfo = []GRef{{Name: sel, Type: "stash"}}
			r.Refs = []string{sel}
			for _, p := range r.Parents[min(1, len(r.Parents)):] {
				hide[p] = true // index / untracked helper commits of a stash are not real history
			}
			if len(r.Parents) > 1 {
				r.Parents = r.Parents[:1]
			}
		} else {
			r.RefInfo, r.Refs = parseDecorations(f[6], o)
		}
		if o.FirstParent && len(r.Parents) > 1 {
			r.Parents = r.Parents[:1]
		}
		rows = append(rows, r)
	}
	kept := rows[:0]
	for _, r := range rows {
		if !hide[r.Hash] {
			kept = append(kept, r)
		}
	}
	rows = kept
	if len(rows) > o.Limit {
		rows, res.Truncate = rows[:o.Limit], true
	}

	if o.InHead && headHash != "" {
		if b, _, err := runGit(repo, 60*time.Second, 16<<20, nil, "rev-list", "--max-count=200000", "HEAD"); err == nil {
			anc := map[string]bool{}
			for _, h := range strings.Fields(string(b)) {
				anc[h] = true
			}
			for i := range rows {
				rows[i].InHead = anc[rows[i].Hash]
			}
		}
	}

	if o.ShowUncommitted && changes > 0 && headHash != "" {
		for _, r := range rows {
			if r.Hash == headHash {
				top := GraphRow{Hash: "*", Parents: []string{headHash}, Author: "*", Subject: "Uncommitted Changes",
					Time: time.Now().Unix(), CTime: time.Now().Unix(), Changes: changes, InHead: true}
				rows = append([]GraphRow{top}, rows...)
				break
			}
		}
	}

	res.Lanes = layoutLanes(rows)
	res.Rows = rows
	return res, nil
}

// layoutLanes assigns each commit a column and the edges to draw. Returns the max lane count.
func layoutLanes(rows []GraphRow) int {
	var lanes []string
	firstEmpty := func() int {
		for i, h := range lanes {
			if h == "" {
				return i
			}
		}
		lanes = append(lanes, "")
		return len(lanes) - 1
	}
	maxLanes := 1
	for ri := range rows {
		row := &rows[ri]
		col := -1
		for i, h := range lanes {
			switch {
			case h == row.Hash:
				if col < 0 {
					col = i
				}
				row.Conv = append(row.Conv, i)
			case h != "":
				row.Pass = append(row.Pass, i)
			}
		}
		if col < 0 {
			col = firstEmpty()
		}
		for _, i := range row.Conv {
			lanes[i] = ""
		}
		for pi, p := range row.Parents {
			t := -1
			for i, h := range lanes {
				if h == p {
					t = i
					break
				}
			}
			if t < 0 {
				if pi == 0 {
					t = col
				} else {
					t = firstEmpty()
				}
				lanes[t] = p
			}
			row.Out = append(row.Out, t)
		}
		row.Col = col
		if len(lanes) > maxLanes {
			maxLanes = len(lanes)
		}
		for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	return maxLanes
}

// gitCommit returns `git show --stat` text (legacy route and MCP tool).
func gitCommit(repo, hash string) (string, error) {
	if !hashRe.MatchString(hash) {
		return "", errors.New("invalid commit hash")
	}
	r, err := cleanRepo(repo)
	if err != nil {
		return "", err
	}
	out, err := git(r, 20*time.Second, "show", "--stat", "--format=fuller", "--no-color", "--no-ext-diff", hash)
	if err != nil {
		return "", err
	}
	if len(out) > 200*1024 {
		out = append(out[:200*1024], []byte("\n... truncated ...")...)
	}
	return string(out), nil
}

// gitDiff returns the full patch of a commit (legacy route and MCP tool).
func gitDiff(repo, hash string) (string, error) {
	if !hashRe.MatchString(hash) {
		return "", errors.New("invalid commit hash")
	}
	r, err := cleanRepo(repo)
	if err != nil {
		return "", err
	}
	out, err := git(r, 30*time.Second, "show", "--format=", "--no-color", "--no-ext-diff", "--no-textconv", "-p", hash)
	if err != nil {
		return "", err
	}
	if len(out) > 400*1024 {
		out = append(out[:400*1024], []byte("\n... truncated ...")...)
	}
	return string(out), nil
}
