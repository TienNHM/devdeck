//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"strings"
)

// dialSystemAgent opens the Windows OpenSSH agent named pipe (or SSH_AUTH_SOCK when it names a
// pipe). The agent protocol is strictly request/response, so plain synchronous file I/O is enough.
func dialSystemAgent() (io.ReadWriteCloser, error) {
	p := `\\.\pipe\openssh-ssh-agent`
	if s := os.Getenv("SSH_AUTH_SOCK"); strings.HasPrefix(s, `\\.\pipe\`) {
		p = s
	}
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return nil, errors.New("ssh-agent is not running (start the \"OpenSSH Authentication Agent\" service)")
	}
	return f, nil
}
