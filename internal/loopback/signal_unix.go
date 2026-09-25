//go:build unix

package loopback

import "syscall"

var syscallTerm = syscall.SIGTERM
