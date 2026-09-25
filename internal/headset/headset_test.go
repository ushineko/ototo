package headset

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

const offline = `HEADSETCONTROL_API_VERSION="1.4"
DEVICE_COUNT=1
DEVICE_0="SteelSeries Arctis Nova Pro Wireless"
DEVICE_0_BATTERY_STATUS="BATTERY_UNAVAILABLE"
DEVICE_0_BATTERY_LEVEL=-1
DEVICE_0_ERROR_COUNT=1
DEVICE_0_ERROR_1_MESSAGE="Device is offline or not responding"
`

const online = `DEVICE_COUNT=1
DEVICE_0="SteelSeries Arctis Nova Pro Wireless"
DEVICE_0_BATTERY_STATUS="BATTERY_AVAILABLE"
DEVICE_0_BATTERY_LEVEL=87
`

// TestAPoweredOffHeadsetIsNotDetected: the dongle answers for a headset that
// is off, with a status and a level of -1, and that is "off" to the
// switcher, not a headset at -1%.
func TestAPoweredOffHeadsetIsNotDetected(t *testing.T) {
	require.False(t, parseEnv(offline).Detected)
	require.False(t, parseEnv("DEVICE_COUNT=0\n").Detected)
	require.False(t, parseEnv("").Detected)
	h := parseEnv(online)
	require.True(t, h.Detected)
	require.Equal(t, "87%", h.Battery)
	h = parseEnv("DEVICE_COUNT=1\nDEVICE_0_BATTERY_STATUS=\"BATTERY_CHARGING\"\nDEVICE_0_BATTERY_LEVEL=40\n")
	require.Equal(t, "40% charging", h.Battery)
}

// TestTheOldShortOutputStillReads: a headsetcontrol without -o prints one
// number, which the original parsed.
func TestTheOldShortOutputStillReads(t *testing.T) {
	require.Equal(t, "55%", parseShort("55\n").Battery)
	require.False(t, parseShort("-1\n").Detected)
	require.False(t, parseShort("Error: Device is offline\n").Detected)
}

// TestBatteryFallsBackToTheShortForm: when -o is refused the tool is old,
// and the old form is tried before giving up.
func TestBatteryFallsBackToTheShortForm(t *testing.T) {
	c := NewWith(func(_ context.Context, args ...string) (string, error) {
		if args[1] == "-o" {
			return "", errors.New("unknown option")
		}
		return "62\n", nil
	})
	require.Equal(t, "62%", c.Battery(context.Background()).Battery)

	absent := NewWith(func(context.Context, ...string) (string, error) { return "", errors.New("not installed") })
	require.False(t, absent.Battery(context.Background()).Detected)
}

// TestIdleIsBounded: 0 is never, 90 is the most the headset takes.
func TestIdleIsBounded(t *testing.T) {
	var got []string
	c := NewWith(func(_ context.Context, args ...string) (string, error) { got = args; return "", nil })
	require.NoError(t, c.SetIdle(context.Background(), 0))
	require.Equal(t, []string{"-i", "0"}, got)
	require.NoError(t, c.SetIdle(context.Background(), 90))
	require.Error(t, c.SetIdle(context.Background(), 91))
	require.Error(t, c.SetIdle(context.Background(), -1))
}

// TestTheLiveToolAnswers only checks that whatever the tool prints parses;
// the headset may be off, absent or on.
func TestTheLiveToolAnswers(t *testing.T) {
	if !Available() {
		t.Skip("headsetcontrol is not installed")
	}
	_ = New().Battery(context.Background())
}
