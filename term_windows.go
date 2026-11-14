//go:build windows

package main

import (
	"errors"
	"os"

	"github.com/UserExistsError/conpty"
)

type localPTY struct{ c *conpty.ConPty }

func (l *localPTY) Read(b []byte) (int, error)  { return l.c.Read(b) }
func (l *localPTY) Write(b []byte) (int, error) { return l.c.Write(b) }
func (l *localPTY) Resize(c, r int) error       { return l.c.Resize(c, r) }
func (l *localPTY) Close() error                { return l.c.Close() }

func startLocal(in termInit) (ptyLike, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, errors.New("ConPTY is not supported on this Windows version")
	}
	cmdline := "powershell.exe -NoLogo"
	if in.Cmd != "" {
		cmdline = "powershell.exe -NoLogo -NoExit -Command " + in.Cmd
	}
	opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(in.Cols, in.Rows)}
	if in.Cwd != "" {
		if st, err := os.Stat(in.Cwd); err == nil && st.IsDir() {
			opts = append(opts, conpty.ConPtyWorkDir(in.Cwd))
		}
	}
	c, err := conpty.Start(cmdline, opts...)
	if err != nil {
		return nil, err
	}
	return &localPTY{c: c}, nil
}
