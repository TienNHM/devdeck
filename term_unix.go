//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/creack/pty"
)

type localPTY struct {
	f    *os.File
	cmd  *exec.Cmd
	once sync.Once
}

func (l *localPTY) Read(b []byte) (int, error)  { return l.f.Read(b) }
func (l *localPTY) Write(b []byte) (int, error) { return l.f.Write(b) }
func (l *localPTY) Resize(c, r int) error {
	return pty.Setsize(l.f, &pty.Winsize{Cols: uint16(c), Rows: uint16(r)})
}

// Close hangs up the terminal (the shell gets SIGHUP), then makes sure the process is gone and reaped.
func (l *localPTY) Close() error {
	l.once.Do(func() {
		_ = l.f.Close()
		done := make(chan struct{})
		go func() { _ = l.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = l.cmd.Process.Kill()
		}
	})
	return nil
}

var (
	shellsOnce sync.Once
	shellsList []termShell
)

// localShells lists $SHELL first, then other common shells found on PATH.
func localShells() []termShell {
	shellsOnce.Do(func() {
		seen := map[string]bool{}
		add := func(p string) {
			if p == "" || seen[p] {
				return
			}
			if st, err := os.Stat(p); err != nil || st.IsDir() {
				return
			}
			seen[p] = true
			n := filepath.Base(p)
			shellsList = append(shellsList, termShell{ID: n, Name: n, Path: p})
		}
		add(os.Getenv("SHELL"))
		for _, n := range []string{"zsh", "bash", "fish", "sh"} {
			if p, err := exec.LookPath(n); err == nil {
				add(p)
			}
		}
	})
	return shellsList
}

func startLocal(in termInit) (ptyLike, error) {
	shells := localShells()
	if len(shells) == 0 {
		return nil, errors.New("no shell found ($SHELL is empty and no sh on PATH)")
	}
	sh := shells[0]
	if in.Shell != "" {
		found := false
		for _, s := range shells {
			if s.ID == in.Shell {
				sh, found = s, true
			}
		}
		if !found {
			return nil, errors.New("shell not available: " + in.Shell)
		}
	}
	cmd := exec.Command(sh.Path, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	if in.Cwd != "" {
		if st, err := os.Stat(in.Cwd); err == nil && st.IsDir() {
			cmd.Dir = in.Cwd
		}
	}
	if cmd.Dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cmd.Dir = home
		}
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(in.Cols), Rows: uint16(in.Rows)})
	if err != nil {
		return nil, err
	}
	if in.Cmd != "" {
		// Typed like the user would, so the shell stays open after the command ends.
		_, _ = f.Write([]byte(in.Cmd + "\n"))
	}
	return &localPTY{f: f, cmd: cmd}, nil
}
