package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/desktop"
)

// fakeBinder is a desktop in memory that records what was asked of it.
type fakeBinder struct {
	supported  bool
	bound      map[string][]string
	volume     bool
	calls      []string
	failBind   map[string]error
	blockDepth int
	maxDepth   int
}

func newFakeBinder(t *testing.T) *fakeBinder {
	t.Helper()
	f := &fakeBinder{supported: true, bound: map[string][]string{}, failBind: map[string]error{}}
	old := binderFor
	binderFor = func(Request) keyBinder { return f }
	t.Cleanup(func() { binderFor = old })
	return f
}

func (f *fakeBinder) Supported(context.Context) bool { return f.supported }
func (f *fakeBinder) RefreshCache(context.Context) error {
	f.calls = append(f.calls, "refresh")
	return nil
}
func (f *fakeBinder) Block(_ context.Context, blocked bool) error {
	if blocked {
		f.calls = append(f.calls, "block")
		f.blockDepth++
	} else {
		f.blockDepth--
		f.calls = append(f.calls, "unblock")
	}
	return nil
}
func (f *fakeBinder) Bind(_ context.Context, key string, args []string) ([]string, error) {
	if f.blockDepth > f.maxDepth {
		f.maxDepth = f.blockDepth
	}
	if f.blockDepth == 0 {
		f.calls = append(f.calls, "UNBLOCKED bind "+key)
	}
	f.calls = append(f.calls, "bind "+key)
	if err := f.failBind[key]; err != nil {
		return nil, err
	}
	f.bound[key] = args
	return nil, nil
}
func (f *fakeBinder) Unbind(_ context.Context, key string) (bool, error) {
	f.calls = append(f.calls, "unbind "+key)
	_, had := f.bound[key]
	delete(f.bound, key)
	return had, nil
}
func (f *fakeBinder) List(context.Context) ([]desktop.Binding, error) {
	out := []desktop.Binding{}
	for k, args := range f.bound {
		out = append(out, desktop.Binding{Entry: "bind-" + k, Key: k, Args: args, Bound: true})
	}
	return out, nil
}
func (f *fakeBinder) InstallVolumeKeys(context.Context) error {
	f.calls = append(f.calls, "volume on")
	f.volume = true
	return nil
}
func (f *fakeBinder) RemoveVolumeKeys(context.Context) (bool, error) {
	f.calls = append(f.calls, "volume off")
	had := f.volume
	f.volume = false
	return had, nil
}
func (f *fakeBinder) VolumeKeysInstalled() (bool, error) { return f.volume, nil }

func keysOf(res HotkeysResult) map[string]config.Hotkey {
	out := map[string]config.Hotkey{}
	for _, h := range res.Hotkeys {
		out[h.Key] = h.Hotkey
	}
	return out
}

// TestKeysOnTheDesktopAreAdoptedOnce: bindings made before the settings
// carried them land in the settings on the first read and not again; one
// that runs something else is reported and left.
func TestKeysOnTheDesktopAreAdoptedOnce(t *testing.T) {
	w := newWorld(t, false)
	f := newFakeBinder(t)
	f.bound["Meta+A"] = []string{"--connect", "AirPods Pro"}
	f.bound["Meta+Num++"] = []string{"--vol-up"}
	f.bound["Meta+X"] = []string{"--something-else"}

	res, err := Hotkeys(context.Background(), HotkeysRequest{w.req()})
	require.NoError(t, err)
	require.True(t, res.Supported)
	require.ElementsMatch(t, []string{"Meta+A", "Meta+Num++"}, res.Adopted)
	require.Equal(t, []string{"bind-Meta+X"}, res.Foreign)
	got := keysOf(res)
	require.Equal(t, config.Hotkey{Key: "Meta+A", Action: config.HotkeyConnect, Device: "AirPods Pro"}, got["Meta+A"])
	require.Equal(t, config.Hotkey{Key: "Meta+Num++", Action: config.HotkeyVolUp}, got["Meta+Num++"])
	for _, h := range res.Hotkeys {
		require.True(t, h.Bound, h.Key)
	}

	res, err = Hotkeys(context.Background(), HotkeysRequest{w.req()})
	require.NoError(t, err)
	require.Empty(t, res.Adopted, "adopted again on the second read")
	saved, _, err := config.Load(w.path)
	require.NoError(t, err)
	require.Len(t, saved.Hotkeys, 2)
}

// TestAKeyIsSetMovedAndRemoved: setting a key binds it; setting another
// key for the same action unbinds the old; a key another action held moves
// to the new one; an empty key removes.
func TestAKeyIsSetMovedAndRemoved(t *testing.T) {
	w := newWorld(t, false)
	f := newFakeBinder(t)
	ctx := context.Background()

	res, err := SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+A", Action: config.HotkeyConnect, Device: "bt:AA"}})
	require.NoError(t, err)
	require.Equal(t, []string{"--connect", "bt:AA"}, f.bound["Meta+A"])
	require.True(t, keysOf(res)["Meta+A"].Action == config.HotkeyConnect)

	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+B", Action: config.HotkeyConnect, Device: "bt:AA"}})
	require.NoError(t, err)
	require.NotContains(t, f.bound, "Meta+A", "the old key stayed bound")
	require.Equal(t, []string{"--connect", "bt:AA"}, f.bound["Meta+B"])

	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+B", Action: config.HotkeyVolUp}})
	require.NoError(t, err)
	require.Equal(t, []string{"--vol-up"}, f.bound["Meta+B"], "the key did not move to the new action")
	saved, _, _ := config.Load(w.path)
	require.Len(t, saved.Hotkeys, 1, "the action that lost its key was kept")
	require.Equal(t, config.HotkeyVolUp, saved.Hotkeys[0].Action)

	res, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "", Action: config.HotkeyVolUp}})
	require.NoError(t, err)
	require.Empty(t, f.bound)
	require.Empty(t, res.Hotkeys)

	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+Q", Action: config.HotkeyConnect}})
	require.Error(t, err, "a connect key without a device was taken")
	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+Wobble", Action: config.HotkeyVolUp}})
	require.Error(t, err, "a key the parser does not know was taken")
}

// TestOffKeepsTheListAndUnbindsEverything: the switch off unbinds every
// key and keeps the settings; on binds them all again; a set while off
// saves and touches no desktop.
func TestOffKeepsTheListAndUnbindsEverything(t *testing.T) {
	w := newWorld(t, false)
	f := newFakeBinder(t)
	ctx := context.Background()
	for _, h := range []config.Hotkey{
		{Key: "Meta+A", Action: config.HotkeyConnect, Device: "bt:AA"},
		{Key: "Meta+Num++", Action: config.HotkeyVolUp},
	} {
		_, err := SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: h})
		require.NoError(t, err)
	}
	res, err := SetHotkeysEnabled(ctx, SetHotkeysEnabledRequest{Request: w.req(), On: false})
	require.NoError(t, err)
	require.Empty(t, f.bound)
	require.False(t, res.Enabled)
	require.Len(t, res.Hotkeys, 2, "the list was lost")
	for _, h := range res.Hotkeys {
		require.False(t, h.Bound)
	}

	f.calls = nil
	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+Z", Action: config.HotkeyConnect, Device: "bt:ZZ"}})
	require.NoError(t, err)
	require.Empty(t, f.calls, "the desktop was touched while the keys are off")

	res, err = SetHotkeysEnabled(ctx, SetHotkeysEnabledRequest{Request: w.req(), On: true})
	require.NoError(t, err)
	require.Len(t, f.bound, 3)
	for _, h := range res.Hotkeys {
		require.True(t, h.Bound, h.Key)
	}
}

// TestOneClickEachWay: Restore stock unbinds the keys and gives the volume
// keys back; Use my keys does the reverse; a desktop failure on one key
// stops nothing else and is reported.
func TestOneClickEachWay(t *testing.T) {
	w := newWorld(t, false)
	f := newFakeBinder(t)
	ctx := context.Background()
	_, err := SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+A", Action: config.HotkeyConnect, Device: "bt:AA"}})
	require.NoError(t, err)
	_, err = SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+Z", Action: config.HotkeyVolDown}})
	require.NoError(t, err)
	_, err = UseMyKeys(ctx, HotkeysRequest{w.req()})
	require.NoError(t, err)
	require.True(t, f.volume)
	require.Equal(t, 0, f.blockDepth, "shortcuts were left blocked")
	require.GreaterOrEqual(t, f.maxDepth, 1, "the batch ran without blocking shortcuts")
	require.NotContains(t, f.calls, "UNBLOCKED bind Meta+A", "a key was bound outside the block")

	res, err := RestoreStockKeys(ctx, HotkeysRequest{w.req()})
	require.NoError(t, err)
	require.False(t, f.volume)
	require.Empty(t, f.bound)
	require.False(t, res.Enabled)
	require.False(t, res.VolumeKeys)
	require.Len(t, res.Hotkeys, 2)

	f.failBind["Meta+A"] = errors.New("held by another shortcut")
	res, err = UseMyKeys(ctx, HotkeysRequest{w.req()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Meta+A")
	require.True(t, f.volume, "the volume keys were skipped after a failed key")
	require.Contains(t, f.bound, "Meta+Z", "the second key was skipped after the first failed")
	require.True(t, res.Enabled)
}

// TestWithoutPlasmaNothingIsBound: unsupported, the settings still hold
// the keys and the desktop is never asked.
func TestWithoutPlasmaNothingIsBound(t *testing.T) {
	w := newWorld(t, false)
	f := newFakeBinder(t)
	f.supported = false
	ctx := context.Background()
	res, err := SetHotkey(ctx, SetHotkeyRequest{Request: w.req(), Hotkey: config.Hotkey{Key: "Meta+A", Action: config.HotkeyConnect, Device: "bt:AA"}})
	require.NoError(t, err)
	require.False(t, res.Supported)
	require.Len(t, res.Hotkeys, 1)
	require.Empty(t, f.calls)
}
