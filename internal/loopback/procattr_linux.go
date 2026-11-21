//go:build linux

package loopback

import "syscall"

// childAttr makes the kernel stop a child when this process dies, so a
// killed ototo leaves no pw-loopback behind to collide with the next one's
// node names.
func childAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
