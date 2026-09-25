/*
Package headset reads a SteelSeries Arctis headset's battery and sets its
idle timeout through headsetcontrol (spec 001 R9.2).

It is a subprocess, and an optional one: the alternative is a HID driver for
one headset model. A machine without the tool, or without the headset, gets
Headset{Detected: false}, which the device model shows as the headset being
off, as the original did.
*/
package headset

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/ototo/internal/devices"
)

// Runner runs headsetcontrol with the given arguments.
type Runner func(ctx context.Context, args ...string) (stdout string, err error)

// Timeout bounds one call: the tool talks to a USB dongle and answers in
// well under a second, and a hang must not stall a tick.
const Timeout = 3 * time.Second

// IdleMax is the longest idle timeout the headset accepts, in minutes.
const IdleMax = 90

// Control is headsetcontrol.
type Control struct {
	run Runner
}

// New uses the headsetcontrol on PATH.
func New() *Control { return &Control{run: runTool} }

// NewWith uses the given runner, for tests.
func NewWith(run Runner) *Control { return &Control{run: run} }

// Available reports whether headsetcontrol can be run at all.
func Available() bool {
	_, err := exec.LookPath("headsetcontrol")
	return err == nil
}

func runTool(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// The program is fixed and the arguments are this package's own.
	out, err := exec.CommandContext(ctx, "headsetcontrol", args...).Output() //nolint:gosec // fixed program, fixed arguments
	if err != nil {
		return string(out), fmt.Errorf("headsetcontrol %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// Battery asks for the battery. Detected is false when the tool is absent,
// the headset is not plugged in, or it is powered off.
func (c *Control) Battery(ctx context.Context) devices.Headset {
	out, err := c.run(ctx, "-b", "-o", "env")
	if err == nil {
		return parseEnv(out)
	}
	// An older headsetcontrol has no -o; its short output is one number.
	out, err = c.run(ctx, "-b", "-c")
	if err != nil {
		return devices.Headset{}
	}
	return parseShort(out)
}

// parseEnv reads the KEY=VALUE output of `-o env` for the first device.
func parseEnv(out string) devices.Headset {
	values := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok {
			values[k] = strings.Trim(v, `"`)
		}
	}
	if values["DEVICE_COUNT"] == "0" || values["DEVICE_COUNT"] == "" {
		return devices.Headset{}
	}
	switch values["DEVICE_0_BATTERY_STATUS"] {
	case "BATTERY_AVAILABLE", "BATTERY_CHARGING":
	default:
		return devices.Headset{}
	}
	level, err := strconv.Atoi(values["DEVICE_0_BATTERY_LEVEL"])
	if err != nil || level < 0 {
		return devices.Headset{}
	}
	h := devices.Headset{Detected: true, Battery: strconv.Itoa(level) + "%"}
	if values["DEVICE_0_BATTERY_STATUS"] == "BATTERY_CHARGING" {
		h.Battery += " charging"
	}
	return h
}

// parseShort reads the one number of the deprecated `-c` output.
func parseShort(out string) devices.Headset {
	level, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || level < 0 {
		return devices.Headset{}
	}
	return devices.Headset{Detected: true, Battery: strconv.Itoa(level) + "%"}
}

// SetIdle sets the idle timeout: 0 never, else 1..IdleMax minutes.
func (c *Control) SetIdle(ctx context.Context, minutes int) error {
	if minutes < 0 || minutes > IdleMax {
		return fmt.Errorf("idle timeout %d is outside 0..%d minutes", minutes, IdleMax)
	}
	_, err := c.run(ctx, "-i", strconv.Itoa(minutes))
	return err
}
