//go:build !windows

package main

import "errors"

func startLocal(in termInit) (ptyLike, error) {
	return nil, errors.New("local terminal is only implemented on Windows so far (PRs welcome); SSH terminals work everywhere")
}
