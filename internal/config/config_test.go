package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(FileEnv, "")
	return home
}

// TestAMissingFileIsTheDefaultDocument: `ototo status` on a machine that has
// never run ototo must answer, not fail on a file that does not exist yet.
func TestAMissingFileIsTheDefaultDocument(t *testing.T) {
	home := sandbox(t)
	cfg, path, err := Load("")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "ototo", "config.json"), path)
	require.Equal(t, Default(), cfg)
	require.True(t, cfg.OSDEnabled)
	require.True(t, cfg.SwitchNotifications)
}

// TestAnOlderFileIsBackfilled: a config.json written by the PyQt6 program
// before osd_enabled existed must read as osd_enabled true, not false.
func TestAnOlderFileIsBackfilled(t *testing.T) {
	sandbox(t)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"device_priority": ["bt:AA:BB:CC:DD:EE:FF", "alsa_output.usb"], "auto_switch": true}`), 0o600))
	cfg, _, err := Load(path)
	require.NoError(t, err)
	require.True(t, cfg.AutoSwitch)
	require.True(t, cfg.OSDEnabled)
	require.True(t, cfg.MoveStreams)
	require.Equal(t, []string{"bt:AA:BB:CC:DD:EE:FF", "alsa_output.usb"}, cfg.DevicePriority)
	require.NotNil(t, cfg.MicLinks)
}

// TestSaveRoundTrips: what Save writes, Load reads back unchanged, and a
// second Save over it replaces rather than appends.
func TestSaveRoundTrips(t *testing.T) {
	sandbox(t)
	path := filepath.Join(t.TempDir(), "deep", "config.json")
	want := Default()
	want.DevicePriority = []string{"bt:00:11:22:33:44:55"}
	want.MicLinks["bt:00:11:22:33:44:55"] = MicDefault
	want.ArctisIdleMinutes = 15
	require.NoError(t, Save(path, want))
	require.NoError(t, Save(path, want))
	got, _, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "a temporary file was left beside the settings")
}

// TestTheEnvironmentOverridesThePath: OTOTO_CONFIG points both front ends at
// the same alternative document.
func TestTheEnvironmentOverridesThePath(t *testing.T) {
	sandbox(t)
	t.Setenv(FileEnv, "~/elsewhere.json")
	path, err := DefaultPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.Getenv("HOME"), "elsewhere.json"), path)
}

// TestAParseErrorNamesTheFile: a damaged file is reported with its path, not
// silently replaced by the defaults, which would lose the device order.
func TestAParseErrorNamesTheFile(t *testing.T) {
	sandbox(t)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	_, _, err := Load(path)
	require.ErrorContains(t, err, path)
}

func TestABareTildeComponentIsRefused(t *testing.T) {
	require.ErrorIs(t, CheckCreatablePath("/tmp/x/~/y"), ErrTildeComponent)
	require.NoError(t, CheckCreatablePath("/tmp/x/y"))
}
