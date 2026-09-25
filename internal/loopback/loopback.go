/*
Package loopback plays a line-in source through the current output
(spec 001 R9.3), two ways, as the original did.

The first is a systemd user unit, audio-loopback.service, which a person
who wants the loopback at every login installs themselves; when it exists,
this package starts and stops it and does nothing else. The second is a
pw-loopback child of the resident process, for a machine without the unit.
Both are subprocesses: the loopback is a PipeWire node, and making one is
what pw-loopback is for.
*/
package loopback

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/ototo/internal/audio"
)

// ServiceName is the systemd user unit the first tier looks for.
const ServiceName = "audio-loopback.service"

// Timeout bounds one systemctl call.
const Timeout = 5 * time.Second

// Mode says which tier is in use.
type Mode string

// Modes.
const (
	ModeNone    Mode = ""        // no line-in source, so nothing to do
	ModeService Mode = "service" // the systemd unit
	ModeDirect  Mode = "direct"  // a pw-loopback child of this process
)

// State is what the Settings card shows.
type State struct {
	// Source is the line-in source in use; "" when the machine has none.
	Source string
	// Candidates are every source with a line input, for a machine with
	// more than one; the setting picks among them.
	Candidates []string
	Mode       Mode
	Active     bool
}

// Runner runs a command and returns its stdout.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Starter starts a long-running command and returns a way to stop it and a
// way to ask whether it still runs.
type Starter func(name string, args ...string) (stop func(), running func() bool, err error)

// Loopback holds the direct child, when there is one.
type Loopback struct {
	run   Runner
	start Starter

	mu      sync.Mutex
	stop    func()
	running func() bool
}

// New uses the real commands.
func New() *Loopback { return &Loopback{run: runCommand, start: startCommand} }

// NewWith uses the given runner and starter, for tests.
func NewWith(run Runner, start Starter) *Loopback { return &Loopback{run: run, start: start} }

/*
LineInSource is the source whose active port is a line input. The original
read the port's type from pactl's JSON; the protocol client does not carry
the type, so this looks at the port's name and description, which every
ALSA card spells with "line".
*/
func LineInSource(sources []audio.Device) string {
	if c := LineInSources(sources); len(c) > 0 {
		return c[0]
	}
	return ""
}

// LineInSources is every source whose active port is a line input, in the
// server's order.
func LineInSources(sources []audio.Device) []string {
	var out []string
	for _, s := range sources {
		if strings.Contains(s.Name, ".monitor") {
			continue
		}
		for _, p := range s.Ports {
			if p.Name != s.ActivePort {
				continue
			}
			name := strings.ToLower(p.Name + " " + p.Description)
			if strings.Contains(name, "line") {
				out = append(out, s.Name)
			}
		}
	}
	return out
}

// Pick is the source to use: the preferred one when it is a candidate, else
// the first. A preferred source that is away falls back rather than
// failing, and the card says which is in use.
func Pick(sources []audio.Device, preferred string) (source string, candidates []string) {
	candidates = LineInSources(sources)
	for _, c := range candidates {
		if c == preferred {
			return c, candidates
		}
	}
	if len(candidates) > 0 {
		return candidates[0], candidates
	}
	return "", candidates
}

// TargetSink is where pw-loopback plays: the JamesDSP sink when there is
// one, so the effects apply, else whatever the default is.
func TargetSink(sinks []audio.Device) string {
	for _, s := range sinks {
		if strings.Contains(strings.ToLower(s.Name), "jamesdsp") {
			return s.Name
		}
	}
	return "@DEFAULT_SINK@"
}

// ServiceInstalled reports whether the systemd unit exists.
func (l *Loopback) ServiceInstalled(ctx context.Context) bool {
	_, err := l.run(ctx, "systemctl", "--user", "cat", ServiceName)
	return err == nil
}

// State reports the tier and whether it is active. source is
// LineInSource's answer.
func (l *Loopback) State(ctx context.Context, source string) State {
	st := State{Source: source}
	if source == "" {
		return st
	}
	if l.ServiceInstalled(ctx) {
		st.Mode = ModeService
		out, err := l.run(ctx, "systemctl", "--user", "is-active", ServiceName)
		st.Active = err == nil && strings.TrimSpace(out) == "active"
		return st
	}
	st.Mode = ModeDirect
	l.mu.Lock()
	st.Active = l.running != nil && l.running()
	l.mu.Unlock()
	return st
}

// ErrNoLineIn is a request to loop a source the machine does not have.
var ErrNoLineIn = errors.New("no line-in source found")

// Set turns the loopback on or off, through whichever tier applies.
func (l *Loopback) Set(ctx context.Context, on bool, source, target string) (State, error) {
	if source == "" {
		return State{}, ErrNoLineIn
	}
	if l.ServiceInstalled(ctx) {
		verb := "stop"
		if on {
			verb = "start"
		}
		if _, err := l.run(ctx, "systemctl", "--user", verb, ServiceName); err != nil {
			return l.State(ctx, source), fmt.Errorf("%s %s: %w", verb, ServiceName, err)
		}
		return l.State(ctx, source), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stop != nil {
		l.stop()
		l.stop, l.running = nil, nil
	}
	if on {
		stop, running, err := l.start("pw-loopback", "-C", source, "-P", target)
		if err != nil {
			return State{Source: source, Mode: ModeDirect}, fmt.Errorf("start pw-loopback: %w", err)
		}
		l.stop, l.running = stop, running
	}
	return State{Source: source, Mode: ModeDirect, Active: on}, nil
}

// Close stops a direct child, if any. Safe to call more than once.
func (l *Loopback) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stop != nil {
		l.stop()
		l.stop, l.running = nil, nil
	}
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// Fixed programs; the arguments are unit names and node names read
	// from the sound server.
	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed programs, arguments from the server
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

// startCommand starts a child and returns a stop that terminates it and
// waits up to three seconds before killing it, as the original did.
func startCommand(name string, args ...string) (func(), func() bool, error) {
	// The child outlives every request: it runs until Set(false) or Close,
	// so it is not bound to a caller's context.
	cmd := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // fixed program, arguments from the server
	if err := cmd.Start(); err != nil {
		return nil, nil, err //nolint:wrapcheck // the caller names the program
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	running := func() bool {
		select {
		case <-done:
			return false
		default:
			return true
		}
	}
	stop := func() {
		if !running() {
			return
		}
		_ = cmd.Process.Signal(syscallTerm)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	return stop, running, nil
}
