package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDesktopStepsAreSwitchesThatReadBack: on writes, off removes, and the
// state afterwards says so. The KWin reload cannot be reached here, so the
// rule step reports that the rule is written but KWin was not told, which
// is the message a machine without KWin gets too.
func TestDesktopStepsAreSwitchesThatReadBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(home, "no-bus"))
	ctx := context.Background()

	st := DesktopStatus(ctx)
	require.False(t, st.Autostart)
	require.False(t, st.IndicatorRule)

	on := true
	st, err := SetDesktop(ctx, SetDesktopRequest{Autostart: &on})
	require.NoError(t, err)
	require.True(t, st.Autostart)

	st, err = SetDesktop(ctx, SetDesktopRequest{IndicatorRule: &on})
	require.Error(t, err, "with no KWin the reload must be reported")
	require.ErrorContains(t, err, "KWin was not told")
	require.True(t, st.IndicatorRule, "the rule was not written")

	off := false
	st, _ = SetDesktop(ctx, SetDesktopRequest{Autostart: &off, IndicatorRule: &off})
	require.False(t, st.Autostart)
	require.False(t, st.IndicatorRule)
}
