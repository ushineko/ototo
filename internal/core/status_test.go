package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
