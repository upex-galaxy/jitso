//go:build !unix

package engine

import "os/exec"

func configureKill(*exec.Cmd) {}

func killGroup(*exec.Cmd) {}
