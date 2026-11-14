//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/UserExistsError/conpty"
)

type localPTY struct{ c *conpty.ConPty }

func (l *localPTY) Read(b []byte) (int, error)  { return l.c.Read(b) }
func (l *localPTY) Write(b []byte) (int, error) { return l.c.Write(b) }
func (l *localPTY) Resize(c, r int) error       { return l.c.Resize(c, r) }
func (l *localPTY) Close() error                { return l.c.Close() }

var (
	shellsOnce sync.Once
	shellsList []termShell
)

// localShells lists the shells found on this machine (looked up once; the first is the default).
func localShells() []termShell {
	shellsOnce.Do(func() {
		add := func(id, name, p string) {
			if p != "" {
				shellsList = append(shellsList, termShell{ID: id, Name: name, Path: p})
			}
		}
		look := func(n string) string { p, _ := exec.LookPath(n); return p }
		add("powershell", "Windows PowerShell", look("powershell.exe"))
		add("pwsh", "PowerShell 7", look("pwsh.exe"))
		add("cmd", "Command Prompt", look("cmd.exe"))
		add("gitbash", "Git Bash", findGitBash())
	})
	return shellsList
}

func findGitBash() string {
	var cands []string
	if g, err := exec.LookPath("git.exe"); err == nil {
		// git.exe lives in <Git>\cmd or <Git>\bin or <Git>\mingw64\bin.
		d := filepath.Dir(g)
		for i := 0; i < 3; i++ {
			cands = append(cands, filepath.Join(d, "bin", "bash.exe"))
			d = filepath.Dir(d)
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if v := os.Getenv(env); v != "" {
			sub := "Git"
			if env == "LOCALAPPDATA" {
				sub = filepath.Join("Programs", "Git")
			}
			cands = append(cands, filepath.Join(v, sub, "bin", "bash.exe"))
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func pickShell(id string) (termShell, error) {
	shells := localShells()
	if len(shells) == 0 {
		return termShell{}, errors.New("no shell found (powershell.exe, pwsh.exe or cmd.exe)")
	}
	if id == "" {
		return shells[0], nil
	}
	for _, s := range shells {
		if s.ID == id {
			return s, nil
		}
	}
	return termShell{}, errors.New("shell not available: " + id)
}

func startLocal(in termInit) (ptyLike, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, errors.New("ConPTY is not supported on this Windows version")
	}
	sh, err := pickShell(in.Shell)
	if err != nil {
		return nil, err
	}
	args := []string{sh.Path}
	typeCmd := false
	switch sh.ID {
	case "powershell", "pwsh":
		args = append(args, "-NoLogo")
		if in.Cmd != "" {
			args = append(args, "-NoExit", "-Command", in.Cmd)
		}
	case "cmd":
		if in.Cmd != "" {
			args = append(args, "/K", in.Cmd)
		}
	case "gitbash":
		args = append(args, "--login", "-i")
		typeCmd = in.Cmd != ""
	}
	for i, a := range args {
		args[i] = syscall.EscapeArg(a)
	}
	opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(in.Cols, in.Rows)}
	if in.Cwd != "" {
		if st, err := os.Stat(in.Cwd); err == nil && st.IsDir() {
			opts = append(opts, conpty.ConPtyWorkDir(in.Cwd))
		}
	}
	c, err := conpty.Start(strings.Join(args, " "), opts...)
	if err != nil {
		return nil, err
	}
	if typeCmd {
		_, _ = c.Write([]byte(in.Cmd + "\r"))
	}
	return &localPTY{c: c}, nil
}
