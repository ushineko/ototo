package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/ushineko/ototo/internal/core"
)

// printStatus is --status as text: one fact per line, then the outputs. Plain
// text with no colour, so it pastes into a bug report as it is.
func printStatus(w io.Writer, res core.StatusResult) {
	fact(w, "ototo", res.Version)
	fact(w, "commit", res.Commit)
	settings := res.ConfigPath
	if !res.ConfigExists {
		settings += " (not written yet; defaults in force)"
	}
	fact(w, "settings", settings)
	fact(w, "auto-switch", onOff(res.Config.AutoSwitch))
	fact(w, "priority", strings.Join(res.Config.DevicePriority, ", "))
	_, _ = fmt.Fprintln(w)

	if res.ServerError != "" {
		fact(w, "sound server", "not reached ("+res.ServerError+")")
		return
	}
	fact(w, "sound server", res.Server.Name+" "+res.Server.Version)
	fact(w, "default output", res.Server.DefaultSink)
	fact(w, "default input", res.Server.DefaultSource)
	fact(w, "inputs", fmt.Sprintf("%d", res.Inputs))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "devices:")
	// The columns are measured rather than guessed: ids run from
	// "bt:AA:BB:CC:DD:EE:FF" to seventy characters of USB descriptor, and a
	// column that fits neither reads as two columns that collided.
	width := 0
	for _, d := range res.Devices {
		width = max(width, len(d.Name))
	}
	for _, d := range res.Devices {
		mark := " "
		if d.Default {
			mark = "*"
		}
		vol := fmt.Sprintf("%3d%%", d.Volume)
		switch {
		case !d.Online:
			vol = "away"
		case d.Mute:
			vol = "muted"
		}
		_, _ = fmt.Fprintf(w, "  %s %-*s  %6s  %s\n", mark, width, d.Name, vol, d.ID)
	}
}

// fact prints one "label: value" line, with the labels aligned. Write errors
// on stdout are not worth failing for: the operation already ran.
func fact(w io.Writer, label, value string) {
	_, _ = fmt.Fprintf(w, "%-16s %s\n", label+":", value)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
