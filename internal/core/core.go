/*
Package core holds every user-facing operation as a headless function
(spec 001 R1).

The rule the whole project turns on: an operation is a function taking a request
struct and returning a result struct, and the callers only render. The window,
the --status flag and the hotkey flags in cmd/ototo therefore run one code path,
and every operation is testable without a display or a sound server.
*/
package core

import (
	"fmt"
)

// Level ranks a log line so a front end can present it by importance. The CLI
// prints debug only under -v; the GUI colours warnings.
type Level int

// Log levels.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String names a level for the CLI's stderr output.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "?"
	}
}

/*
Events is how an operation talks back to whichever front end started it.

The field may be nil and every call site goes through logf, so a caller that
does not care writes nothing: `core.Status(ctx, core.StatusRequest{})` works.
*/
type Events struct {
	Log func(level Level, msg string)
}

func (e Events) logf(level Level, format string, args ...any) {
	if e.Log == nil {
		return
	}
	e.Log(level, fmt.Sprintf(format, args...))
}

/*
Request is what every operation carries: which config and which sound server.

It is embedded rather than passed separately so that each operation has exactly
one argument, which is what makes the front ends mechanical.
*/
type Request struct {
	// ConfigPath overrides the settings file; "" uses the default.
	ConfigPath string
	// Server overrides the sound server; "" uses $PULSE_SERVER or the
	// session's own socket.
	Server string
	// Events receives log lines. May be the zero value.
	Events Events
	// Probes read the Bluetooth adapter and the headset; nil reads the
	// machine (DefaultProbes). A test supplies its own.
	Probes *Probes
}

func (r Request) probes() Probes {
	if r.Probes == nil {
		return DefaultProbes()
	}
	return *r.Probes
}
