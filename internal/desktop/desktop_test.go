package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/glance/kwin"
)

func sandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

// TestAutostartIsOneFileUnderTheUsersConfig: install writes it, remove takes
// it away, and both say what they found.
func TestAutostartIsOneFileUnderTheUsersConfig(t *testing.T) {
	dir := sandbox(t)
	on, err := AutostartInstalled()
	require.NoError(t, err)
	require.False(t, on)
	require.NoError(t, InstallAutostart())
	on, err = AutostartInstalled()
	require.NoError(t, err)
	require.True(t, on)
	body, err := os.ReadFile(filepath.Join(dir, ".config", "autostart", AppID+".desktop"))
	require.NoError(t, err)
	require.Contains(t, string(body), "Exec=ototo\n")
	removed, err := RemoveAutostart()
	require.NoError(t, err)
	require.True(t, removed)
	removed, err = RemoveAutostart()
	require.NoError(t, err)
	require.False(t, removed)
}

// TestTheIndicatorRuleKeysOnTheTitle: the main window's own rule, if the
// user has one, must survive the indicator's, and the indicator's must
// carry everything R8 measured as necessary.
func TestTheIndicatorRuleKeysOnTheTitle(t *testing.T) {
	sandbox(t)
	require.NoError(t, kwin.Install(kwin.Rule{AppID: AppID, Opacity: 90}))
	require.NoError(t, kwin.Install(IndicatorRule()))
	on, err := RuleInstalled()
	require.NoError(t, err)
	require.True(t, on)
	main, ok, err := kwin.Lookup(AppID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 90, main.Opacity)
	require.False(t, main.NoBorder)
	r := IndicatorRule()
	require.True(t, r.NoBorder && r.AlwaysOnTop && r.SkipTaskbar && r.SkipSwitcher && r.SkipPager && r.NoFocus)
}

// TestThePlacementScriptTouchesOnlyOurWindow: the class and the title are
// quoted into the script, so a title with a quote in it cannot become code.
func TestThePlacementScriptTouchesOnlyOurWindow(t *testing.T) {
	js := placeScriptFor(AppID, `ototo-indicator"; evil()`)
	require.Contains(t, js, `const cls = "io.ushineko.ototo";`)
	require.Contains(t, js, `const title = "ototo-indicator\"; evil()";`)
	require.Contains(t, js, "w.resourceClass !== cls || w.caption !== title")
	require.Contains(t, js, "workspace.cursorPos")
	require.Equal(t, 1, strings.Count(js, "frameGeometry ="))
}
