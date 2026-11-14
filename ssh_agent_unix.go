//go:build !windows

package main

import (
	"errors"
	"io"
	"net"
	"os"
	"time"
)

// dialSystemAgent connects to the agent socket named by SSH_AUTH_SOCK.
func dialSystemAgent() (io.ReadWriteCloser, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("ssh-agent is not running (SSH_AUTH_SOCK is not set)")
	}
	return net.DialTimeout("unix", sock, 2*time.Second)
}
