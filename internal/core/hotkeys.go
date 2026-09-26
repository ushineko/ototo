package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/desktop"
)

/*
Hotkeys (spec 002): the keys of the person's own, in the settings file and
bound on the desktop to match it. Every operation here is over a keyBinder,
which is the desktop, an in-memory one for the demo server, or a test's.
*/

// keyBinder is what the hotkeys need from the desktop.
type keyBinder interface {
	Supported(ctx context.Context) bool
	// Block suspends and resumes shortcut dispatch around a batch of
	// changes, so a keypress cannot crash the compositor mid-registration.
	Block(ctx context.Context, blocked bool) error
	Bind(ctx context.Context, key string, args []string) (released []string, err error)
	Unbind(ctx context.Context, key string) (bool, error)
	List(ctx context.Context) ([]desktop.Binding, error)
	InstallVolumeKeys(ctx context.Context) error
	RemoveVolumeKeys(ctx context.Context) (bool, error)
	VolumeKeysInstalled() (bool, error)
}

type desktopKeys struct{}

func (desktopKeys) Supported(ctx context.Context) bool { return desktop.Supported(ctx) }
func (desktopKeys) Block(ctx context.Context, blocked bool) error {
	return desktop.SetShortcutsBlocked(ctx, blocked)
}
func (desktopKeys) Bind(ctx context.Context, key string, args []string) ([]string, error) {
	return desktop.Bind(ctx, key, args)
}
func (desktopKeys) Unbind(ctx context.Context, key string) (bool, error) {
	return desktop.Unbind(ctx, key)
}
func (desktopKeys) List(ctx context.Context) ([]desktop.Binding, error) {
	list, err := desktop.ListBindings()
	if err != nil {
		return nil, err
	}
	// The on-disk file lags a bind, so bound-ness is read from the live
	// registry, which is right at once.
	for i := range list {
		list[i].Bound = desktop.BindingIsLive(ctx, list[i].Entry, list[i].Key)
	}
	return list, nil
}
func (desktopKeys) InstallVolumeKeys(ctx context.Context) error {
	return desktop.InstallVolumeKeys(ctx)
}
func (desktopKeys) RemoveVolumeKeys(ctx context.Context) (bool, error) {
	return desktop.RemoveVolumeKeys(ctx)
}
func (desktopKeys) VolumeKeysInstalled() (bool, error) { return desktop.VolumeKeysInstalled() }

// demoKeys is the demo server's desktop: bindings in memory, so the window
// over invented devices can be worked and screenshotted without touching
// the machine.
type demoKeys struct {
	mu      sync.Mutex
	bound   map[string][]string
	volume  bool
	matched bool
}

var demoDesktop = &demoKeys{bound: map[string][]string{}, volume: true}

// match makes the demo desktop hold the settings' keys, once, the way a
// real desktop holds what an earlier run bound.
func (d *demoKeys) match(cfg config.Config) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.matched {
		return
	}
	d.matched = true
	if !cfg.HotkeysEnabled {
		return
	}
	for _, h := range cfg.Hotkeys {
		d.bound[h.Key] = hotkeyArgs(h)
	}
}

func (d *demoKeys) Supported(context.Context) bool {
	// OTOTO_DEMO_NO_HOTKEYS renders the section as it looks on a desktop
	// without KDE's global shortcut service, for the screenshot harness.
	return os.Getenv("OTOTO_DEMO_NO_HOTKEYS") == ""
}
func (d *demoKeys) Block(context.Context, bool) error { return nil }
func (d *demoKeys) Bind(_ context.Context, key string, args []string) ([]string, error) {
	if _, err := desktop.ParseKey(key); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bound[key] = args
	return nil, nil
}
func (d *demoKeys) Unbind(_ context.Context, key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, had := d.bound[key]
	delete(d.bound, key)
	return had, nil
}
func (d *demoKeys) List(context.Context) ([]desktop.Binding, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]desktop.Binding, 0, len(d.bound))
	for k, args := range d.bound {
		out = append(out, desktop.Binding{Entry: k, Key: k, Args: args, Bound: true})
	}
	return out, nil
}
func (d *demoKeys) InstallVolumeKeys(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.volume = true
	return nil
}
func (d *demoKeys) RemoveVolumeKeys(context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	had := d.volume
	d.volume = false
	return had, nil
}
func (d *demoKeys) VolumeKeysInstalled() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.volume, nil
}

// blocked runs fn with the binder's shortcut dispatch suspended, when the
// desktop is one that can bind, so a batch of registry changes cannot be
// interrupted by a keypress. The unblock always runs. A block that fails
// is logged and the batch goes on: the risk is the compositor's, not a
// reason to refuse the person their keys.
func blocked(ctx context.Context, b keyBinder, ev Events, fn func() []error) []error {
	if !b.Supported(ctx) {
		return nil // no desktop to bind on; the settings still hold the keys
	}
	if err := b.Block(ctx, true); err != nil {
		ev.logf(LevelWarn, "could not suspend shortcuts for the change: %v", err)
	} else {
		defer func() {
			if err := b.Block(ctx, false); err != nil {
				ev.logf(LevelWarn, "could not resume shortcuts after the change: %v", err)
			}
		}()
	}
	return fn()
}

// binderFor picks the desktop for a request; a test replaces it.
var binderFor = func(req Request) keyBinder {
	if audio.IsDemo(req.Server) {
		return demoDesktop
	}
	return desktopKeys{}
}

// HotkeyState is one hotkey and whether the desktop has it.
type HotkeyState struct {
	config.Hotkey
	// Bound says kglobalaccel has the key on ototo's entry now.
	Bound bool
}

// HotkeysResult is the section's state.
type HotkeysResult struct {
	// Supported says keys can be bound here (KDE Plasma); without it the
	// list is still the settings, and nothing is bound.
	Supported bool
	// Enabled is the settings' switch.
	Enabled bool
	// VolumeKeys says Volume Up and Volume Down run ototo.
	VolumeKeys bool
	Hotkeys    []HotkeyState
	// Adopted names keys found bound on the desktop and added to the
	// settings by this read (spec 002 R1.3).
	Adopted []string
	// Foreign names ototo binding entries that run something this program
	// does not know; they are left as they are.
	Foreign []string
}

// HotkeysRequest reads the state.
type HotkeysRequest struct{ Request }

// Hotkeys reads the settings and the desktop, adopts bindings the settings
// do not know, and reports which keys are bound.
func Hotkeys(ctx context.Context, req HotkeysRequest) (HotkeysResult, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return HotkeysResult{}, err
	}
	b := binderFor(req.Request)
	if d, ok := b.(*demoKeys); ok {
		d.match(cfg)
	}
	res := HotkeysResult{Supported: b.Supported(ctx), Enabled: cfg.HotkeysEnabled}
	bound := map[string]bool{}
	if res.Supported {
		list, err := b.List(ctx)
		if err != nil {
			return res, err
		}
		for _, bd := range list {
			if bd.Bound {
				bound[bd.Key] = true
			}
			h, ok := hotkeyFromArgs(bd.Key, bd.Args)
			if !ok {
				res.Foreign = append(res.Foreign, bd.Entry)
				continue
			}
			if !bd.Bound || hasKey(cfg.Hotkeys, bd.Key) {
				continue
			}
			cfg.Hotkeys = append(cfg.Hotkeys, h)
			res.Adopted = append(res.Adopted, bd.Key)
		}
		if len(res.Adopted) > 0 {
			if err := config.Save(path, cfg); err != nil {
				return res, err
			}
			req.Events.logf(LevelInfo, "adopted %d key(s) found on the desktop into the settings", len(res.Adopted))
		}
		if res.VolumeKeys, err = b.VolumeKeysInstalled(); err != nil {
			return res, err
		}
	}
	for _, h := range cfg.Hotkeys {
		res.Hotkeys = append(res.Hotkeys, HotkeyState{Hotkey: h, Bound: bound[h.Key]})
	}
	return res, nil
}

// SetHotkeyRequest sets the key for one action, or removes it with an
// empty key.
type SetHotkeyRequest struct {
	Request
	Hotkey config.Hotkey
}

/*
SetHotkey saves the key and, when the keys are enabled and the desktop can
bind, makes the desktop match: the action's old key is unbound, another
action holding the new key loses it (a key runs one thing), and the new key
is bound. A desktop error leaves the settings changed and comes back with
the state.
*/
func SetHotkey(ctx context.Context, req SetHotkeyRequest) (HotkeysResult, error) {
	h := req.Hotkey
	if !slices.Contains(config.HotkeyActions, h.Action) {
		return HotkeysResult{}, fmt.Errorf("%q is not an action a key can run", h.Action)
	}
	if h.Action == config.HotkeyConnect && h.Device == "" {
		return HotkeysResult{}, errors.New("a connect key needs a device")
	}
	if h.Action != config.HotkeyConnect {
		h.Device = ""
	}
	if h.Key != "" {
		if err := ValidHotkey(h.Key); err != nil {
			return HotkeysResult{}, err
		}
	}
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return HotkeysResult{}, err
	}
	var unbind []string
	kept := cfg.Hotkeys[:0:0]
	for _, x := range cfg.Hotkeys {
		switch {
		case x.Action == h.Action && x.Device == h.Device:
			if x.Key != h.Key {
				unbind = append(unbind, x.Key)
			}
		case h.Key != "" && x.Key == h.Key:
			// Moved to this action; the binding is overwritten below.
		default:
			kept = append(kept, x)
		}
	}
	if h.Key != "" {
		kept = append(kept, h)
	}
	cfg.Hotkeys = kept
	if err := config.Save(path, cfg); err != nil {
		return HotkeysResult{}, err
	}
	b := binderFor(req.Request)
	var errs []error
	if cfg.HotkeysEnabled {
		errs = blocked(ctx, b, req.Events, func() []error {
			return applyHotkey(ctx, b, unbind, h, req.Events)
		})
	}
	return finish(ctx, req.Request, errs)
}

// applyHotkey unbinds the keys the change frees and binds the new one, with
// no block of its own: the caller holds it. Bind is skipped when the key is
// empty (a removal).
func applyHotkey(ctx context.Context, b keyBinder, unbind []string, h config.Hotkey, ev Events) []error {
	var errs []error
	for _, k := range unbind {
		if _, err := b.Unbind(ctx, k); err != nil {
			errs = append(errs, fmt.Errorf("unbind %s: %w", k, err))
		} else {
			ev.logf(LevelInfo, "%s unbound", k)
		}
	}
	if h.Key != "" {
		released, err := b.Bind(ctx, h.Key, hotkeyArgs(h))
		for _, r := range released {
			ev.logf(LevelInfo, "%s released from %s", h.Key, r)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("bind %s: %w", h.Key, err))
		} else {
			ev.logf(LevelInfo, "%s runs: ototo %s", h.Key, joinArgs(hotkeyArgs(h)))
		}
	}
	return errs
}

// SetHotkeysEnabledRequest turns every key on or off.
type SetHotkeysEnabledRequest struct {
	Request
	On bool
}

// SetHotkeysEnabled saves the switch and binds or unbinds every hotkey,
// each one whether or not an earlier one failed, under one block.
func SetHotkeysEnabled(ctx context.Context, req SetHotkeysEnabledRequest) (HotkeysResult, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return HotkeysResult{}, err
	}
	cfg.HotkeysEnabled = req.On
	if err := config.Save(path, cfg); err != nil {
		return HotkeysResult{}, err
	}
	b := binderFor(req.Request)
	errs := blocked(ctx, b, req.Events, func() []error {
		return applyEnabled(ctx, b, cfg, req.On, req.Events)
	})
	return finish(ctx, req.Request, errs)
}

// applyEnabled binds or unbinds every hotkey, with no block of its own.
func applyEnabled(ctx context.Context, b keyBinder, cfg config.Config, on bool, ev Events) []error {
	var errs []error
	for _, h := range cfg.Hotkeys {
		if on {
			if _, err := b.Bind(ctx, h.Key, hotkeyArgs(h)); err != nil {
				errs = append(errs, fmt.Errorf("bind %s: %w", h.Key, err))
			}
		} else if _, err := b.Unbind(ctx, h.Key); err != nil {
			errs = append(errs, fmt.Errorf("unbind %s: %w", h.Key, err))
		}
	}
	if on {
		ev.logf(LevelInfo, "%d key(s) bound", len(cfg.Hotkeys))
	} else {
		ev.logf(LevelInfo, "%d key(s) unbound and given back", len(cfg.Hotkeys))
	}
	return errs
}

// applyVolume installs or removes the volume keys, with no block of its own.
func applyVolume(ctx context.Context, b keyBinder, on bool, ev Events) []error {
	if on {
		if err := b.InstallVolumeKeys(ctx); err != nil {
			return []error{fmt.Errorf("volume keys: %w", err)}
		}
		ev.logf(LevelInfo, "the volume keys run ototo")
		return nil
	}
	if _, err := b.RemoveVolumeKeys(ctx); err != nil {
		return []error{fmt.Errorf("volume keys: %w", err)}
	}
	ev.logf(LevelInfo, "the volume keys are given back")
	return nil
}

// finish reads the state after a batch and joins the batch's errors with
// the read's.
func finish(ctx context.Context, req Request, errs []error) (HotkeysResult, error) {
	res, err := Hotkeys(ctx, HotkeysRequest{req})
	if err != nil {
		errs = append(errs, err)
	}
	return res, errors.Join(errs...)
}

// RestoreStockKeys is one click back to the desktop's own keys: every
// hotkey unbound and given back, and the volume keys given back, in one
// block. The settings keep the list.
func RestoreStockKeys(ctx context.Context, req HotkeysRequest) (HotkeysResult, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return HotkeysResult{}, err
	}
	cfg.HotkeysEnabled = false
	if err := config.Save(path, cfg); err != nil {
		return HotkeysResult{}, err
	}
	b := binderFor(req.Request)
	errs := blocked(ctx, b, req.Events, func() []error {
		out := applyEnabled(ctx, b, cfg, false, req.Events)
		return append(out, applyVolume(ctx, b, false, req.Events)...)
	})
	return finish(ctx, req.Request, errs)
}

// UseMyKeys is one click back to the person's own: every hotkey bound and
// the volume keys taken, in one block.
func UseMyKeys(ctx context.Context, req HotkeysRequest) (HotkeysResult, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return HotkeysResult{}, err
	}
	cfg.HotkeysEnabled = true
	if err := config.Save(path, cfg); err != nil {
		return HotkeysResult{}, err
	}
	b := binderFor(req.Request)
	errs := blocked(ctx, b, req.Events, func() []error {
		out := applyEnabled(ctx, b, cfg, true, req.Events)
		return append(out, applyVolume(ctx, b, true, req.Events)...)
	})
	return finish(ctx, req.Request, errs)
}

// SetVolumeKeys is the volume-keys switch on its own.
func SetVolumeKeys(ctx context.Context, req SetHotkeysEnabledRequest) (HotkeysResult, error) {
	b := binderFor(req.Request)
	errs := blocked(ctx, b, req.Events, func() []error {
		return applyVolume(ctx, b, req.On, req.Events)
	})
	return finish(ctx, req.Request, errs)
}

// ValidHotkey says whether text is a key this program can bind, for an
// entry's validator.
func ValidHotkey(text string) error {
	_, err := desktop.ParseKey(text)
	return err
}

// hotkeyArgs is what ototo runs for a hotkey.
func hotkeyArgs(h config.Hotkey) []string {
	switch h.Action {
	case config.HotkeyConnect:
		return []string{"--connect", h.Device}
	case config.HotkeyVolUp:
		return []string{"--vol-up"}
	case config.HotkeyVolDown:
		return []string{"--vol-down"}
	}
	return nil
}

// hotkeyFromArgs reads a binding's arguments back into a hotkey, or says
// the binding runs something else.
func hotkeyFromArgs(key string, args []string) (config.Hotkey, bool) {
	switch {
	case len(args) == 2 && args[0] == "--connect" && args[1] != "":
		return config.Hotkey{Key: key, Action: config.HotkeyConnect, Device: args[1]}, true
	case len(args) == 1 && args[0] == "--vol-up":
		return config.Hotkey{Key: key, Action: config.HotkeyVolUp}, true
	case len(args) == 1 && args[0] == "--vol-down":
		return config.Hotkey{Key: key, Action: config.HotkeyVolDown}, true
	}
	return config.Hotkey{}, false
}

func hasKey(list []config.Hotkey, key string) bool {
	return slices.ContainsFunc(list, func(h config.Hotkey) bool { return h.Key == key })
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
