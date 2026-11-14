package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Every write operation goes through gitActions: a fixed whitelist, strict argument validation,
// exec with an argument slice, and one global lock so concurrent clicks cannot fight over index.lock.

var gitOpMu sync.Mutex

const (
	tShort = 60 * time.Second
	tNet   = 5 * time.Minute
)

type aargs map[string]any

func (a aargs) s(k string) string {
	v, _ := a[k].(string)
	return strings.TrimSpace(v)
}
func (a aargs) raw(k string) string { v, _ := a[k].(string); return v }
func (a aargs) b(k string) bool     { v, _ := a[k].(bool); return v }
func (a aargs) n(k string) int {
	switch v := a[k].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}
func (a aargs) list(k string) []string {
	var out []string
	if arr, ok := a[k].([]any); ok {
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

// run executes git and returns combined stdout+stderr (git prints progress and conflicts on either).
func run(repo string, timeout time.Duration, args ...string) (string, error) {
	out, se, err := runGit(repo, timeout, 2<<20, nil, args...)
	comb := strings.TrimSpace(strings.TrimSpace(string(out)) + "\n" + se)
	if err != nil {
		if comb == "" {
			comb = err.Error()
		}
		return comb, errors.New(comb)
	}
	return comb, nil
}

func needRev(a aargs, k string) (string, error) {
	v := a.s(k)
	if !validRev(v) {
		return "", fmt.Errorf("invalid %s", k)
	}
	return v, nil
}
func needRef(a aargs, k string) (string, error) {
	v := a.s(k)
	if !validRef(v) {
		return "", fmt.Errorf("invalid %s name", k)
	}
	return v, nil
}
func needRemote(a aargs, k string) (string, error) {
	v := a.s(k)
	if !validRemote(v) {
		return "", fmt.Errorf("invalid remote name")
	}
	return v, nil
}

func mainline(a aargs) ([]string, error) {
	m := a.n("parent")
	if m < 0 || m > 16 {
		return nil, errors.New("invalid parent number")
	}
	if m > 0 {
		return []string{"-m", strconv.Itoa(m)}, nil
	}
	return nil, nil
}

var gitActions map[string]func(repo string, a aargs) (string, error)

func init() {
	gitActions = map[string]func(string, aargs) (string, error){
		"branch.create":          actBranchCreate,
		"branch.checkout":        actBranchCheckout,
		"branch.checkout_remote": actCheckoutRemote,
		"branch.delete":          actBranchDelete,
		"branch.delete_remote":   actDeleteRemoteBranch,
		"branch.rename":          actBranchRename,
		"branch.merge":           actMerge,
		"commit.merge":           actMerge,
		"branch.pull":            actPull,
		"branch.push":            actPush,
		"branch.rebase":          actRebase,
		"commit.rebase":          actRebase,
		"branch.fetch_into":      actFetchInto,
		"reset":                  actReset,
		"tag.add":                actTagAdd,
		"tag.delete":             actTagDelete,
		"tag.push":               actTagPush,
		"commit.checkout":        actCommitCheckout,
		"commit.cherrypick":      actCherryPick,
		"commit.revert":          actRevert,
		"commit.drop":            actDrop,
		"stash.push":             actStashPush,
		"stash.apply":            func(r string, a aargs) (string, error) { return stashOp(r, a, "apply") },
		"stash.pop":              func(r string, a aargs) (string, error) { return stashOp(r, a, "pop") },
		"stash.drop":             func(r string, a aargs) (string, error) { return stashOp(r, a, "drop") },
		"stash.branch":           actStashBranch,
		"uncommitted.reset":      actUncommittedReset,
		"uncommitted.clean":      actUncommittedClean,
		"remote.fetch":           actFetch,
		"remote.add":             actRemoteAdd,
		"remote.edit":            actRemoteEdit,
		"remote.delete":          actRemoteDelete,
		"remote.prune":           actRemotePrune,
		"op.abort":               func(r string, a aargs) (string, error) { return opControl(r, a, "--abort") },
		"op.continue":            func(r string, a aargs) (string, error) { return opControl(r, a, "--continue") },
		"commit":                 actCommit,
	}
}

// execGitAction validates and runs one whitelisted action. It is the only write entry point.
func execGitAction(repoIn, action string, a aargs) (string, error) {
	repo, err := cleanRepo(repoIn)
	if err != nil {
		return "", err
	}
	fn, ok := gitActions[action]
	if !ok {
		return "", errors.New("unknown action: " + action)
	}
	if a == nil {
		a = aargs{}
	}
	gitOpMu.Lock()
	defer gitOpMu.Unlock()
	return fn(repo, a)
}

// ------------------------------------------------------------ branches

func actBranchCreate(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	hash, err := needRev(a, "hash")
	if err != nil {
		return "", err
	}
	args := []string{"branch"}
	if a.b("force") {
		args = append(args, "-f")
	}
	args = append(args, name, hash)
	out, err := run(repo, tShort, args...)
	if err != nil {
		return out, err
	}
	if a.b("checkout") {
		o2, err := run(repo, tShort, "checkout", name, "--")
		return out + "\n" + o2, err
	}
	return out, nil
}

func actBranchCheckout(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "checkout", name, "--")
}

func actCheckoutRemote(repo string, a aargs) (string, error) {
	remoteBranch, err := needRef(a, "remoteBranch")
	if err != nil {
		return "", err
	}
	local, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "checkout", "-b", local, "--track", remoteBranch)
}

func actBranchDelete(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	flag := "-d"
	if a.b("force") {
		flag = "-D"
	}
	out, err := run(repo, tShort, "branch", flag, name)
	if err != nil {
		return out, err
	}
	if rm := a.s("remote"); rm != "" {
		if !validRemote(rm) {
			return out, errors.New("invalid remote name")
		}
		o2, err := run(repo, tNet, "push", rm, "--delete", name)
		return out + "\n" + o2, err
	}
	return out, nil
}

func actDeleteRemoteBranch(repo string, a aargs) (string, error) {
	rm, err := needRemote(a, "remote")
	if err != nil {
		return "", err
	}
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tNet, "push", rm, "--delete", name)
}

func actBranchRename(repo string, a aargs) (string, error) {
	old, err := needRef(a, "old")
	if err != nil {
		return "", err
	}
	nw, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "branch", "-m", old, nw)
}

func commitIfSquash(repo string, a aargs, out string) (string, error) {
	if a.b("squash") && !a.b("noCommit") {
		o2, err := run(repo, tShort, "commit", "--no-edit")
		return out + "\n" + o2, err
	}
	return out, nil
}

func actMerge(repo string, a aargs) (string, error) {
	target, err := needRev(a, "target")
	if err != nil {
		return "", err
	}
	if a.b("noFF") && a.b("squash") {
		return "", errors.New("--no-ff and --squash cannot be combined")
	}
	args := []string{"merge", "--no-edit"}
	if a.b("noFF") {
		args = append(args, "--no-ff")
	}
	if a.b("squash") {
		args = append(args, "--squash")
	}
	if a.b("noCommit") && !a.b("squash") {
		args = append(args, "--no-commit")
	}
	args = append(args, target)
	out, err := run(repo, tShort, args...)
	if err != nil {
		return out, err
	}
	return commitIfSquash(repo, a, out)
}

func actPull(repo string, a aargs) (string, error) {
	rm, err := needRemote(a, "remote")
	if err != nil {
		return "", err
	}
	br, err := needRef(a, "branch")
	if err != nil {
		return "", err
	}
	if a.b("noFF") && a.b("squash") {
		return "", errors.New("--no-ff and --squash cannot be combined")
	}
	args := []string{"pull", "--no-edit"}
	if a.b("noFF") {
		args = append(args, "--no-ff")
	}
	if a.b("squash") {
		args = append(args, "--squash")
	}
	args = append(args, rm, br)
	out, err := run(repo, tNet, args...)
	if err != nil {
		return out, err
	}
	return commitIfSquash(repo, a, out)
}

func actPush(repo string, a aargs) (string, error) {
	br, err := needRef(a, "branch")
	if err != nil {
		return "", err
	}
	remotes := a.list("remotes")
	if len(remotes) == 0 {
		return "", errors.New("no remote selected")
	}
	mode := a.s("mode")
	var all []string
	for _, rm := range remotes {
		if !validRemote(rm) {
			return strings.Join(all, "\n"), errors.New("invalid remote name")
		}
		args := []string{"push"}
		if a.b("setUpstream") {
			args = append(args, "-u")
		}
		switch mode {
		case "force-with-lease":
			args = append(args, "--force-with-lease")
		case "force":
			args = append(args, "--force")
		case "", "normal":
		default:
			return "", errors.New("invalid push mode")
		}
		args = append(args, rm, br)
		out, err := run(repo, tNet, args...)
		all = append(all, out)
		if err != nil {
			return strings.Join(all, "\n"), fmt.Errorf("push to %s failed: %w", rm, err)
		}
	}
	return strings.Join(all, "\n"), nil
}

func actRebase(repo string, a aargs) (string, error) {
	onto, err := needRev(a, "onto")
	if err != nil {
		return "", err
	}
	args := []string{"rebase"}
	if a.b("ignoreDate") {
		args = append(args, "--ignore-date")
	}
	return run(repo, tShort, append(args, onto)...)
}

func actFetchInto(repo string, a aargs) (string, error) {
	rm, err := needRemote(a, "remote")
	if err != nil {
		return "", err
	}
	rb, err := needRef(a, "remoteBranch")
	if err != nil {
		return "", err
	}
	lb, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	args := []string{"fetch"}
	if a.b("force") {
		args = append(args, "--force")
	}
	return run(repo, tNet, append(args, rm, rb+":"+lb)...)
}

func actReset(repo string, a aargs) (string, error) {
	to, err := needRev(a, "to")
	if err != nil {
		return "", err
	}
	mode := a.s("mode")
	if !validModes[mode] {
		return "", errors.New("invalid reset mode")
	}
	return run(repo, tShort, "reset", "--"+mode, to, "--")
}

// ------------------------------------------------------------ tags

func actTagAdd(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	hash, err := needRev(a, "hash")
	if err != nil {
		return "", err
	}
	args := []string{"tag"}
	if !a.b("lightweight") {
		msg := a.raw("message")
		if strings.TrimSpace(msg) == "" || len(msg) > 20000 {
			return "", errors.New("an annotated tag needs a message (or choose lightweight)")
		}
		args = append(args, "-a", "-m", msg)
	}
	out, err := run(repo, tShort, append(args, name, hash)...)
	if err != nil {
		return out, err
	}
	if rm := a.s("pushRemote"); rm != "" {
		if !validRemote(rm) {
			return out, errors.New("invalid remote name")
		}
		o2, err := run(repo, tNet, "push", rm, "refs/tags/"+name)
		return out + "\n" + o2, err
	}
	return out, nil
}

func actTagDelete(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	out, err := run(repo, tShort, "tag", "-d", name)
	if err != nil {
		return out, err
	}
	if rm := a.s("remote"); rm != "" {
		if !validRemote(rm) {
			return out, errors.New("invalid remote name")
		}
		o2, err := run(repo, tNet, "push", rm, ":refs/tags/"+name)
		return out + "\n" + o2, err
	}
	return out, nil
}

func actTagPush(repo string, a aargs) (string, error) {
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	rm, err := needRemote(a, "remote")
	if err != nil {
		return "", err
	}
	return run(repo, tNet, "push", rm, "refs/tags/"+name)
}

// ------------------------------------------------------------ commits

func actCommitCheckout(repo string, a aargs) (string, error) {
	hash, err := needRev(a, "hash")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "checkout", "--detach", hash)
}

func actCherryPick(repo string, a aargs) (string, error) {
	hash, err := needRev(a, "hash")
	if err != nil {
		return "", err
	}
	ml, err := mainline(a)
	if err != nil {
		return "", err
	}
	args := []string{"cherry-pick"}
	if a.b("record") {
		args = append(args, "-x")
	}
	if a.b("noCommit") {
		args = append(args, "--no-commit")
	}
	args = append(args, ml...)
	return run(repo, tShort, append(args, hash)...)
}

func actRevert(repo string, a aargs) (string, error) {
	hash, err := needRev(a, "hash")
	if err != nil {
		return "", err
	}
	ml, err := mainline(a)
	if err != nil {
		return "", err
	}
	args := append([]string{"revert", "--no-edit"}, ml...)
	return run(repo, tShort, append(args, hash)...)
}

func actDrop(repo string, a aargs) (string, error) {
	hash := a.s("hash")
	if !hashRe.MatchString(hash) {
		return "", errors.New("invalid commit hash")
	}
	if _, _, err := runGit(repo, 10*time.Second, 256, nil, "merge-base", "--is-ancestor", hash, "HEAD"); err != nil {
		return "", errors.New("only commits that are ancestors of HEAD can be dropped")
	}
	parents, err := gitOut(repo, 10*time.Second, "rev-list", "--parents", "-n", "1", hash)
	if err != nil {
		return "", err
	}
	if n := len(strings.Fields(parents)); n != 2 {
		return "", errors.New("only ordinary (non-merge, non-root) commits can be dropped")
	}
	return run(repo, tShort, "rebase", "--onto", hash+"^", hash)
}

// ------------------------------------------------------------ stash / uncommitted

func actStashPush(repo string, a aargs) (string, error) {
	args := []string{"stash", "push"}
	if a.b("includeUntracked") {
		args = append(args, "--include-untracked")
	}
	if msg := a.raw("message"); strings.TrimSpace(msg) != "" {
		if len(msg) > 2000 {
			return "", errors.New("message too long")
		}
		args = append(args, "-m", msg)
	}
	return run(repo, tShort, args...)
}

func stashOp(repo string, a aargs, op string) (string, error) {
	sel := a.s("selector")
	if !stashRe.MatchString(sel) {
		return "", errors.New("invalid stash")
	}
	args := []string{"stash", op}
	if (op == "apply" || op == "pop") && a.b("reinstateIndex") {
		args = append(args, "--index")
	}
	return run(repo, tShort, append(args, sel)...)
}

func actStashBranch(repo string, a aargs) (string, error) {
	sel := a.s("selector")
	if !stashRe.MatchString(sel) {
		return "", errors.New("invalid stash")
	}
	name, err := needRef(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "stash", "branch", name, sel)
}

func actUncommittedReset(repo string, a aargs) (string, error) {
	mode := a.s("mode")
	if mode != "mixed" && mode != "hard" {
		return "", errors.New("invalid reset mode")
	}
	return run(repo, tShort, "reset", "--"+mode, "HEAD", "--")
}

func actUncommittedClean(repo string, a aargs) (string, error) {
	args := []string{"clean", "-f"}
	if a.b("directories") {
		args = append(args, "-d")
	}
	return run(repo, tShort, args...)
}

// ------------------------------------------------------------ remotes

func actFetch(repo string, a aargs) (string, error) {
	args := []string{"fetch"}
	if rm := a.s("remote"); rm != "" {
		if !validRemote(rm) {
			return "", errors.New("invalid remote name")
		}
		args = append(args, rm)
	} else {
		args = append(args, "--all")
	}
	if a.b("prune") {
		args = append(args, "--prune")
	}
	if a.b("pruneTags") {
		args = append(args, "--prune-tags")
	}
	return run(repo, tNet, args...)
}

func actRemoteAdd(repo string, a aargs) (string, error) {
	name, err := needRemote(a, "name")
	if err != nil {
		return "", err
	}
	url := a.s("url")
	if !validRemoteURL(url) {
		return "", errors.New("invalid remote URL")
	}
	out, err := run(repo, tShort, "remote", "add", name, url)
	if err != nil {
		return out, err
	}
	if pu := a.s("pushUrl"); pu != "" {
		if !validRemoteURL(pu) {
			return out, errors.New("invalid push URL")
		}
		if o2, err := run(repo, tShort, "remote", "set-url", "--push", name, pu); err != nil {
			return out + "\n" + o2, err
		}
	}
	if a.b("fetch") {
		o3, err := run(repo, tNet, "fetch", name)
		return out + "\n" + o3, err
	}
	return out, nil
}

func actRemoteEdit(repo string, a aargs) (string, error) {
	old, err := needRemote(a, "old")
	if err != nil {
		return "", err
	}
	name, err := needRemote(a, "name")
	if err != nil {
		return "", err
	}
	url := a.s("url")
	if !validRemoteURL(url) {
		return "", errors.New("invalid remote URL")
	}
	var all []string
	if old != name {
		o, err := run(repo, tShort, "remote", "rename", old, name)
		all = append(all, o)
		if err != nil {
			return strings.Join(all, "\n"), err
		}
	}
	o, err := run(repo, tShort, "remote", "set-url", name, url)
	all = append(all, o)
	if err != nil {
		return strings.Join(all, "\n"), err
	}
	if pu := a.s("pushUrl"); pu != "" && pu != url {
		if !validRemoteURL(pu) {
			return strings.Join(all, "\n"), errors.New("invalid push URL")
		}
		o, err = run(repo, tShort, "remote", "set-url", "--push", name, pu)
	} else {
		// no separate push URL: make push follow the fetch URL again
		o, err = run(repo, tShort, "remote", "set-url", "--push", name, url)
	}
	all = append(all, o)
	return strings.Join(all, "\n"), err
}

func actRemoteDelete(repo string, a aargs) (string, error) {
	name, err := needRemote(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tShort, "remote", "remove", name)
}

func actRemotePrune(repo string, a aargs) (string, error) {
	name, err := needRemote(a, "name")
	if err != nil {
		return "", err
	}
	return run(repo, tNet, "remote", "prune", name)
}

// ------------------------------------------------------------ in-progress operations

func opControl(repo string, a aargs, flag string) (string, error) {
	op := a.s("op")
	switch op {
	case "merge":
		if flag == "--continue" {
			return run(repo, tShort, "commit", "--no-edit")
		}
		return run(repo, tShort, "merge", "--abort")
	case "rebase", "cherry-pick", "revert":
		return run(repo, tShort, op, flag)
	}
	return "", errors.New("unknown operation")
}

// ------------------------------------------------------------ commit

// actCommit commits the index (optionally staging everything first, optionally amending HEAD).
// The message goes to git on stdin (`-F -`) and is never placed in the argument list.
func actCommit(repo string, a aargs) (string, error) {
	msg := strings.ReplaceAll(a.raw("message"), "\r\n", "\n")
	if strings.ContainsRune(msg, 0) || len(msg) > 20000 {
		return "", errors.New("invalid commit message")
	}
	amend := a.b("amend")
	if strings.TrimSpace(msg) == "" && !amend {
		return "", errors.New("a commit message is required")
	}
	var all []string
	if a.b("all") {
		o, err := run(repo, tShort, "add", "-A", "--", ".")
		all = append(all, o)
		if err != nil {
			return strings.TrimSpace(strings.Join(all, "\n")), err
		}
	}
	args := []string{"commit"}
	if amend {
		args = append(args, "--amend")
	}
	var stdin io.Reader
	if strings.TrimSpace(msg) == "" {
		args = append(args, "--no-edit") // amend keeping the previous message
	} else {
		args = append(args, "-F", "-")
		stdin = strings.NewReader(msg)
	}
	out, se, err := runGit(repo, tShort, 2<<20, stdin, args...)
	comb := strings.TrimSpace(strings.TrimSpace(string(out)) + "\n" + se)
	all = append(all, comb)
	res := strings.TrimSpace(strings.Join(all, "\n"))
	if err != nil {
		if res == "" {
			res = err.Error()
		}
		return res, errors.New(res)
	}
	return res, nil
}
