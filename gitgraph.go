package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type GraphRow struct {
	Hash    string   `json:"h"`
	Parents []string `json:"p"`
	Author  string   `json:"a"`
	Time    int64    `json:"t"`
	Refs    []string `json:"r,omitempty"`
	Subject string   `json:"s"`
	Col     int      `json:"col"`
	Pass    []int    `json:"pass"` // lanes passing straight through this row
	Conv    []int    `json:"conv"` // lanes converging into this commit from above
	Out     []int    `json:"out"`  // lanes the parents occupy below this commit
}

type GraphResult struct {
	Rows     []GraphRow `json:"rows"`
	Lanes    int        `json:"lanes"`
	Branch   string     `json:"branch"`
	Dirty    bool       `json:"dirty"`
	Truncate bool       `json:"truncated"`
}

var hashRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func git(repo string, timeout time.Duration, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo, "-c", "core.quotepath=false"}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
		}
		return out.Bytes(), nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return nil, errors.New("git timed out")
	}
}

func gitGraph(repo string, limit int) (*GraphResult, error) {
	if limit <= 0 || limit > 5000 {
		limit = 300
	}
	out, err := git(repo, 20*time.Second, "log", "--all", "--topo-order",
		"-n", strconv.Itoa(limit+1),
		"--format=%H%x1f%P%x1f%an%x1f%at%x1f%D%x1f%s")
	if err != nil {
		return nil, err
	}
	res := &GraphResult{}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > limit {
		lines, res.Truncate = lines[:limit], true
	}
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
	maxLanes := 0
	for _, ln := range lines {
		f := strings.Split(ln, "\x1f")
		if len(f) < 6 {
			continue
		}
		row := GraphRow{Hash: f[0], Author: f[2], Subject: f[5]}
		if f[1] != "" {
			row.Parents = strings.Fields(f[1])
		}
		row.Time, _ = strconv.ParseInt(f[3], 10, 64)
		if f[4] != "" {
			for _, r := range strings.Split(f[4], ", ") {
				row.Refs = append(row.Refs, strings.TrimSpace(r))
			}
		}
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
		res.Rows = append(res.Rows, row)
	}
	res.Lanes = maxLanes
	if b, err := git(repo, 5*time.Second, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		res.Branch = strings.TrimSpace(string(b))
	}
	if b, err := git(repo, 10*time.Second, "status", "--porcelain"); err == nil {
		res.Dirty = len(bytes.TrimSpace(b)) > 0
	}
	return res, nil
}

func gitCommit(repo, hash string) (string, error) {
	if !hashRe.MatchString(hash) {
		return "", errors.New("invalid commit hash")
	}
	out, err := git(repo, 20*time.Second, "show", "--stat", "--format=fuller", "--no-color", hash)
	if err != nil {
		return "", err
	}
	if len(out) > 200*1024 {
		out = append(out[:200*1024], []byte("\n... truncated ...")...)
	}
	return string(out), nil
}

func gitDiff(repo, hash string) (string, error) {
	if !hashRe.MatchString(hash) {
		return "", errors.New("invalid commit hash")
	}
	out, err := git(repo, 30*time.Second, "show", "--format=", "--no-color", "-p", hash)
	if err != nil {
		return "", err
	}
	if len(out) > 400*1024 {
		out = append(out[:400*1024], []byte("\n... truncated ...")...)
	}
	return string(out), nil
}
