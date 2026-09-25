/*
Package instance keeps one ototo running and lets a second invocation talk
to it (spec 001 D10).

The window hides to the tray, so the next click on the launcher starts a
second process, and a hotkey starts one every time a key is pressed. The
first process takes a lock and listens on a Unix socket in the runtime
directory; a later one connects, writes one line, reads one line and exits.
The original did this with a Qt local socket named ag_audio_source_switcher
and the messages SHOW, VOL_UP and VOL_DOWN.

The lock is an flock, which the kernel drops when the holder dies, so a
crash cannot leave a stale lock; a stale socket file is removed before
listening.
*/
package instance

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// DirEnv overrides the directory the lock and socket live in, for tests.
const DirEnv = "OTOTO_RUNTIME_DIR"

const (
	socketName = "ototo.sock"
	lockName   = "ototo.lock"
	// AskTimeout bounds a request to the running instance; a request that
	// switches devices can take a Bluetooth connect's worth of time.
	AskTimeout = 15 * time.Second
)

// ErrRunning says another instance holds the lock.
var ErrRunning = errors.New("another ototo is running")

// Dir is where the lock and the socket live: $OTOTO_RUNTIME_DIR, else
// $XDG_RUNTIME_DIR, else the temporary directory.
func Dir() string {
	for _, env := range []string{DirEnv, "XDG_RUNTIME_DIR"} {
		if d := os.Getenv(env); d != "" {
			return d
		}
	}
	return os.TempDir()
}

// Handler answers one request line with one reply line.
type Handler func(request string) (reply string)

// Server is the running instance's end.
type Server struct {
	lock *os.File
	l    net.Listener
	path string
}

// Listen takes the lock and starts answering. ErrRunning means another
// instance has it, and the caller should Ask that one instead.
func Listen(handle Handler) (*Server, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the runtime directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // the runtime directory
	if err != nil {
		return nil, fmt.Errorf("open the lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, ErrRunning
	}
	path := filepath.Join(dir, socketName)
	_ = os.Remove(path) // a stale socket from a crashed instance
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	s := &Server{lock: lock, l: l, path: path}
	go s.serve(handle)
	return s, nil
}

func (s *Server) serve(handle Handler) {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			return // closed
		}
		go func() {
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(AskTimeout))
			line, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil {
				return
			}
			reply := handle(strings.TrimSpace(line))
			_, _ = fmt.Fprintln(conn, reply)
		}()
	}
}

// Close stops answering and releases the lock.
func (s *Server) Close() {
	_ = s.l.Close()
	_ = os.Remove(s.path)
	_ = s.lock.Close() // closing drops the flock
}

// Ask sends one request to the running instance and returns its reply. ok
// is false when no instance answered, which is when the caller acts on its
// own.
func Ask(request string) (reply string, ok bool) {
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(context.Background(), "unix", filepath.Join(Dir(), socketName))
	if err != nil {
		return "", false
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(AskTimeout))
	if _, err := fmt.Fprintln(conn, request); err != nil {
		return "", false
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(line), true
}
