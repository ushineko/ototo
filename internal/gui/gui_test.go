package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
)

/*
The window's tests run headless.

Fyne's test driver draws into memory and runs fyne.Do inline, so everything here
works with no DISPLAY and no WAYLAND_DISPLAY, which is the requirement, since
`make test` runs on machines that have neither.

What is deliberately not tested here is the sections' layout. The tests pin
behaviour instead, and each one names the bug it prevents.
*/

// testUI is a window with no window: the program's state over a headless
// shell, with HOME and the XDG directories pointed at throwaway paths, and the
// sound server pointed at a socket that does not exist so no test reaches the
// developer's own.
func testUI(t *testing.T) *ui {
	t.Helper()
	home := fynetest.Sandbox(t)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "run"))
	t.Setenv("PULSE_SERVER", "unix:"+filepath.Join(home, "no-server"))
	app := test.NewApp()
	t.Cleanup(app.Quit)
	u := &ui{version: "test", commit: "0000000"}
	u.sh = shell.Headless(app, u.shellOptions(Options{}))
	win := test.NewWindow(widget.NewLabel(""))
	u.sh.Window = win
	t.Cleanup(win.Close)
	return u
}

// TestSectionNamesNeedsNoApp: the flag help lists these while parsing flags,
// before there is a Fyne app to construct a theme icon against.
func TestSectionNamesNeedsNoApp(t *testing.T) {
	names := SectionNames()
	require.Equal(t, []string{"Outputs", "Appearance", "About"}, names)
	for _, n := range names {
		require.Containsf(t, sectionBuilders(), n, "%q is advertised but has no section", n)
	}
	require.Len(t, sectionBuilders(), len(names), "a section exists that the navigation never shows")
}

// TestAMachineWithNoSoundServerStillDrawsOutputs: the section says why the
// list is empty rather than reading forever or leaving the window blank.
func TestAMachineWithNoSoundServerStillDrawsOutputs(t *testing.T) {
	u := testUI(t)
	u.loadStatus() // headless, so this runs inline
	require.True(t, u.statusOK)
	require.False(t, u.loading)
	require.NotEmpty(t, u.status.ServerError)

	texts := fynetest.Texts(u.buildOutputs())
	require.Contains(t, strings.Join(texts, "\n"), "No sound server answered")

	segs := u.statusSegments()
	require.Contains(t, strings.Join(fynetest.Texts(segs[1]), "\n"), "not reached")
}

// TestALoadInProgressIsNotStartedTwice: a rebuild while the server is being
// read must not queue a second read behind the first.
func TestALoadInProgressIsNotStartedTwice(t *testing.T) {
	u := testUI(t)
	u.loading = true
	u.loadStatus()
	require.False(t, u.statusOK, "a second read ran while the first was in progress")
}

// TestOutputRowsSayWhatAPersonWouldAsk: the mark, the state and the volume
// cells are the three facts a glance needs, asserted through the pure
// function because the table builds cells only on a canvas.
func TestOutputRowsSayWhatAPersonWouldAsk(t *testing.T) {
	cells := outputCells(core.Output{Name: "alsa_output.x", Description: "Speakers", Default: true, Connected: true, Volume: 42})
	require.Equal(t, []string{"playing", "Speakers", "connected", "42%", "alsa_output.x"}, cells)

	cells = outputCells(core.Output{Name: "bluez_output.y", Mute: true})
	require.Equal(t, []string{"", "bluez_output.y", "disconnected", "muted", "bluez_output.y"}, cells)
}

// TestTheDefaultOutputIsNamedForAPerson: the status bar says "Speakers", not
// the ALSA sink name, when the server gave a description.
func TestTheDefaultOutputIsNamedForAPerson(t *testing.T) {
	res := core.StatusResult{
		Server:  audio.Server{DefaultSink: "alsa_output.x"},
		Outputs: []core.Output{{Name: "alsa_output.x", Description: "Speakers", Default: true}},
	}
	require.Equal(t, "Speakers", defaultOutputText(res))
	res.Outputs = nil
	require.Equal(t, "alsa_output.x", defaultOutputText(res))
}
