//go:build !linux

package loopback

import "syscall"

func childAttr() *syscall.SysProcAttr { return nil }
