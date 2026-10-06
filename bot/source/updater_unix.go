//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

func runUpdateHelper(path string) error {
	return errors.New("Online installation is supported on Windows")
}

func hideUpdateHelper(cmd *exec.Cmd) {}
