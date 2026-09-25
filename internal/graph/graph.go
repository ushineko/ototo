/*
Package graph rewires the PipeWire graph for JamesDSP (spec 001 R6).

JamesDSP registers a virtual sink and a filter node; applications play into
the sink, the filter processes, and the filter's output ports are linked to
the playback ports of one hardware sink. To switch outputs without losing
the effects, this package moves those links.

That is PipeWire's own graph, which the PulseAudio protocol cannot reach, so
this is the one place the program runs a subprocess for audio: pw-link, with
the same three invocations and the same text parsing as the original. The
parsers are pure functions over the text, tested over captured shapes; the
runner is a function so the tests supply the text.
*/
package graph

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Runner runs pw-link with the given arguments and returns its stdout.
type Runner func(ctx context.Context, args ...string) (string, error)

// Timeout bounds one pw-link call. The tool answers in milliseconds; a hang
// is a PipeWire that has stopped, and a switch must not wait on it.
const Timeout = 3 * time.Second

// ErrNoOutputs and ErrNoInputs are why a relink did not happen.
var (
	ErrNoOutputs    = errors.New("JamesDSP has no output ports in the graph")
	ErrNoInputs     = errors.New("the target sink has no playback ports in the graph")
	ErrNotAvailable = errors.New("pw-link is not installed")
)

// Graph is the PipeWire graph as pw-link shows it.
type Graph struct {
	run Runner
}

// New uses the pw-link on PATH.
func New() *Graph { return &Graph{run: runPwLink} }

// NewWith uses the given runner, for tests.
func NewWith(run Runner) *Graph { return &Graph{run: run} }

// Available reports whether pw-link can be run at all.
func Available() bool {
	_, err := exec.LookPath("pw-link")
	return err == nil
}

func runPwLink(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("pw-link"); err != nil {
		return "", ErrNotAvailable
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// The program is fixed and the arguments are port names read from
	// pw-link itself or sink names read from the sound server; nothing here
	// comes from a shell or a user's text.
	out, err := exec.CommandContext(ctx, "pw-link", args...).Output() //nolint:gosec // fixed program, arguments from the graph
	if err != nil {
		return "", fmt.Errorf("pw-link %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// JamesDSPOutputs lists the filter's output ports, sorted, or nothing when
// JamesDSP is not in the graph.
func (g *Graph) JamesDSPOutputs(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "-o")
	if err != nil {
		return nil, err
	}
	return parseOutputs(out), nil
}

// PlaybackPorts lists a sink's playback ports, sorted.
func (g *Graph) PlaybackPorts(ctx context.Context, sink string) ([]string, error) {
	out, err := g.run(ctx, "-i")
	if err != nil {
		return nil, err
	}
	return parsePlayback(out, sink), nil
}

// Target is the sink JamesDSP plays into right now: "" when it is floating
// (outputs with no links) or absent (no outputs).
func (g *Graph) Target(ctx context.Context) (string, error) {
	outs, err := g.JamesDSPOutputs(ctx)
	if err != nil || len(outs) == 0 {
		return "", err
	}
	links, err := g.run(ctx, "-l")
	if err != nil {
		return "", err
	}
	return linkedSink(links, outs[0]), nil
}

/*
Relink moves JamesDSP's output to sink: every existing playback link of every
output port is removed, then the sorted outputs are linked to the sorted
playback ports pairwise, which is what pairs FL with FL.

The unlinks are not checked, as the original did not check them: a link that
is already gone is not a failure, and the links that matter are the ones made
next. A missing output or input port is a failure the caller reports and
recovers from by switching the hardware sink directly.
*/
func (g *Graph) Relink(ctx context.Context, sink string) error {
	outs, err := g.JamesDSPOutputs(ctx)
	if err != nil {
		return err
	}
	if len(outs) == 0 {
		return ErrNoOutputs
	}
	ins, err := g.PlaybackPorts(ctx, sink)
	if err != nil {
		return err
	}
	if len(ins) == 0 {
		return fmt.Errorf("%w: %s", ErrNoInputs, sink)
	}
	links, err := g.run(ctx, "-l")
	if err != nil {
		return err
	}
	for _, out := range outs {
		for _, target := range linkedPorts(links, out) {
			_, _ = g.run(ctx, "-d", out, target)
		}
	}
	for i := 0; i < len(outs) && i < len(ins); i++ {
		if _, err := g.run(ctx, outs[i], ins[i]); err != nil {
			return fmt.Errorf("link %s to %s: %w", outs[i], ins[i], err)
		}
	}
	return nil
}

// parseOutputs keeps the lines of `pw-link -o` that are JamesDSP output
// ports, by the three markers the original matched.
func parseOutputs(text string) []string {
	var ports []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "jdsp_") && strings.Contains(line, "JamesDsp") && strings.Contains(line, ":output_") {
			ports = append(ports, strings.TrimSpace(line))
		}
	}
	sort.Strings(ports)
	return ports
}

// parsePlayback keeps the lines of `pw-link -i` that are the named sink's
// playback ports.
func parsePlayback(text, sink string) []string {
	var ports []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, sink) && strings.Contains(line, ":playback_") {
			ports = append(ports, strings.TrimSpace(line))
		}
	}
	sort.Strings(ports)
	return ports
}

/*
linkedPorts reads `pw-link -l` and returns the playback ports that the named
output port is linked to.

The listing puts each port on its own line, and each of its links on the
lines below, indented and marked with |-> for an outgoing link. A line that
is not indented starts the next port.
*/
func linkedPorts(text, port string) []string {
	var targets []string
	capture := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.TrimSpace(line) == port:
			capture = true
		case capture && strings.HasPrefix(line, "  |->"):
			target := strings.TrimSpace(strings.TrimPrefix(line, "  |->"))
			if strings.Contains(target, ":playback_") {
				targets = append(targets, target)
			}
		case capture && !strings.HasPrefix(line, "  "):
			capture = false
		}
	}
	return targets
}

// linkedSink is the sink that owns the first playback port an output is
// linked to, or "" when it is linked to nothing.
func linkedSink(text, port string) string {
	for _, target := range linkedPorts(text, port) {
		if sink, _, ok := strings.Cut(target, ":playback_"); ok {
			return sink
		}
	}
	return ""
}
