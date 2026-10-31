package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
)

// TestStatusAnswersWithNoSoundServer: on a machine with no server (a CI
// runner, a container) the command reports why and still shows the settings,
// rather than exiting with an error that hides everything else.
func TestStatusAnswersWithNoSoundServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "run"))
	t.Setenv("PULSE_SERVER", "unix:"+filepath.Join(home, "nothing-here"))

	var warned []string
	res, err := Status(context.Background(), StatusRequest{Request: Request{
		Probes: &Probes{},
		Events: Events{Log: func(l Level, m string) {
			if l == LevelWarn {
				warned = append(warned, m)
			}
		}},
	}})
	require.NoError(t, err)
	require.False(t, res.ConfigExists)
	require.True(t, res.Config.OSDEnabled)
	require.NotEmpty(t, res.ServerError)
	require.Empty(t, res.Devices)
	require.NotEmpty(t, warned, "the reason the server was not reached goes to the log")
}

// TestStatusRemembersWhatAnAwayDeviceWasCalled (spec 004 R2.4): a device in
// the order is named, while its sink is there, into the settings file, and
// named that way after the sink is gone. A steady machine does not rewrite
// the file on every read.
func TestStatusRemembersWhatAnAwayDeviceWasCalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	const dac = "alsa_output.usb-Example_DAC-00.analog-stereo"
	cfgPath := filepath.Join(home, "config.json")
	cfg := config.Default()
	cfg.DevicePriority = []string{dac, "alsa_output.gone-before-this"}
	cfg.DeviceNames = map[string]string{"alsa_output.left-the-order": "Old"}
	require.NoError(t, config.Save(cfgPath, cfg))

	present := filepath.Join(home, "present.json")
	require.NoError(t, os.WriteFile(present, []byte(demoJSON), 0o600))
	res, err := Status(context.Background(), StatusRequest{Request: Request{Server: audio.DemoPrefix + present, ConfigPath: cfgPath}})
	require.NoError(t, err)
	require.Equal(t, map[string]string{dac: "Example DAC"}, res.Config.DeviceNames)
	saved, _, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Equal(t, map[string]string{dac: "Example DAC"}, saved.DeviceNames, "the name was not written to the file")

	before, err := os.Stat(cfgPath)
	require.NoError(t, err)
	_, err = Status(context.Background(), StatusRequest{Request: Request{Server: audio.DemoPrefix + present, ConfigPath: cfgPath}})
	require.NoError(t, err)
	after, err := os.Stat(cfgPath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime(), "nothing changed and the file was rewritten")

	gone := filepath.Join(home, "gone.json")
	require.NoError(t, os.WriteFile(gone, []byte(`{"sinks": []}`), 0o600))
	res, err = Status(context.Background(), StatusRequest{Request: Request{Server: audio.DemoPrefix + gone, ConfigPath: cfgPath}})
	require.NoError(t, err)
	require.Equal(t, "Example DAC [Disconnected]", res.Devices[0].Name)
	require.Equal(t, "alsa_output.gone-before-this [Disconnected]", res.Devices[1].Name)
	saved, _, err = config.Load(cfgPath)
	require.NoError(t, err)
	require.Equal(t, map[string]string{dac: "Example DAC"}, saved.DeviceNames, "an away device's name was forgotten")
}
