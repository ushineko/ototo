package gui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
)

/*
The Hotkeys section (spec 002 R4): a key per device, keys for the volume
steps, the volume keys switch, and one click each way between the person's
keys and the desktop's own. On a desktop that cannot bind keys it is a
page saying how to bind the commands by hand.
*/

// keyEntryWidth fits "Ctrl+Alt+Shift+F12" with room.
const keyEntryWidth float32 = 200

func (u *ui) buildHotkeys() fyne.CanvasObject {
	heading := widgets.Heading("Hotkeys", "Keys of your own that connect a device or step the volume.")
	if !u.hotkeysOK || !u.statusOK {
		return container.NewVBox(heading, widgets.Note("Reading the keys…", fd.StatusInfo))
	}
	res := u.hotkeys
	if !res.Supported {
		return container.NewVBox(heading, u.hotkeysPage())
	}
	return container.NewVBox(heading, u.everythingCard(), u.deviceKeysCard(), u.volumeKeysCard())
}

// loadHotkeys is the arrival hook: the keys and, when it is not there yet,
// the device list they are for.
func (u *ui) loadHotkeys() {
	if !u.statusOK {
		u.loadStatus()
	}
	u.sh.Load("Reading the keys...", func(ctx context.Context) error {
		res, err := core.Hotkeys(ctx, core.HotkeysRequest{Request: u.request()})
		fyne.Do(func() {
			if err == nil {
				u.hotkeys, u.hotkeysOK = res, true
			}
		})
		return err
	})
}

// hotkeyFor is the hotkey for an action and device, matching an adopted
// key's device text the way --connect would.
func (u *ui) hotkeyFor(action string, dev devices.Device) (core.HotkeyState, bool) {
	for _, h := range u.hotkeys.Hotkeys {
		if h.Action != action {
			continue
		}
		if action != config.HotkeyConnect {
			return h, true
		}
		if h.Device == dev.ID {
			return h, true
		}
		if d, ok := core.Resolve(u.status.Devices, h.Device); ok && d.ID == dev.ID {
			return h, true
		}
	}
	return core.HotkeyState{}, false
}

// keyRow is one row of the keys form: the label and, beside it, the key
// entry with its state. The rows go into one form layout so the entries
// line up in a column whatever the labels' widths. The entry commits on
// Enter; a key the parser does not know is marked before that.
func (u *ui) keyRow(label string, have core.HotkeyState, ok bool, commit func(key string)) (name, field fyne.CanvasObject) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("none")
	entry.Validator = func(text string) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return core.ValidHotkey(text)
	}
	if ok {
		entry.SetText(have.Key)
	}
	entry.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text != "" && core.ValidHotkey(text) != nil {
			return
		}
		if (ok && text == have.Key) || (!ok && text == "") {
			return
		}
		commit(text)
	}
	state := widget.NewLabel("")
	switch {
	case !ok:
	case !u.hotkeys.Enabled:
		state.SetText("saved, keys are off")
	case have.Bound:
		state.SetText("bound")
	default:
		state.SetText("not bound")
	}
	return widget.NewLabel(label), container.NewHBox(widgets.FixedWidth(entry, keyEntryWidth), state)
}

// keysForm lays label and field pairs out as a grid: labels in one column,
// entries in the next.
func keysForm(pairs ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewFormLayout(), pairs...)
}

func (u *ui) deviceKeysCard() fyne.CanvasObject {
	var pairs []fyne.CanvasObject
	for _, d := range u.status.Devices {
		d := d
		have, ok := u.hotkeyFor(config.HotkeyConnect, d)
		device := d.ID
		if ok {
			device = have.Device // as it was adopted, which --connect resolves
		}
		name, field := u.keyRow(devices.Plain(d.Name), have, ok, func(key string) {
			u.setHotkey(config.Hotkey{Key: key, Action: config.HotkeyConnect, Device: device})
		})
		pairs = append(pairs, name, field)
	}
	body := keysForm(pairs...)
	if len(pairs) == 0 {
		body = widgets.DimWrapped("No devices yet. The list is the one in Outputs.")
	}
	return widgets.Card("A key per device",
		widgets.DimWrapped("The key connects the device when it is away and plays on it, as \"Switch to\" does. "+
			"Type the key as System Settings writes it, Meta+A or Ctrl+Alt+F5, and press Enter; empty and Enter "+
			"removes it."),
		body,
	)
}

func (u *ui) volumeKeysCard() fyne.CanvasObject {
	up, upOK := u.hotkeyFor(config.HotkeyVolUp, devices.Device{})
	down, downOK := u.hotkeyFor(config.HotkeyVolDown, devices.Device{})
	keys := check("Use the volume keys (Volume Up, Volume Down) for ototo", u.hotkeys.VolumeKeys, func(on bool) {
		u.hotkeysOp("Changing the volume keys...", func(ctx context.Context) (core.HotkeysResult, error) {
			return core.SetVolumeKeys(ctx, core.SetHotkeysEnabledRequest{Request: u.request(), On: on})
		})
	})
	return widgets.Card("Volume",
		widgets.WithTip(keys, "Volume Up and Volume Down run ototo, which changes the volume and shows the "+
			"indicator. What held the keys before is recorded and released; turned off, the keys go back to it."),
		widgets.DimWrapped("Keys of your own for the same two steps, for a keyboard without volume keys."),
		keysForm(slices.Concat(
			pair(u.keyRow("Volume up", up, upOK, func(key string) {
				u.setHotkey(config.Hotkey{Key: key, Action: config.HotkeyVolUp})
			})),
			pair(u.keyRow("Volume down", down, downOK, func(key string) {
				u.setHotkey(config.Hotkey{Key: key, Action: config.HotkeyVolDown})
			})),
		)...),
	)
}

func (u *ui) everythingCard() fyne.CanvasObject {
	res := u.hotkeys
	bound := 0
	for _, h := range res.Hotkeys {
		if h.Bound {
			bound++
		}
	}
	var line fyne.CanvasObject
	switch {
	case !res.Enabled && len(res.Hotkeys) == 0 && !res.VolumeKeys:
		line = widgets.Note("Stock keys: nothing is bound and no key is saved.", fd.StatusInfo)
	case res.Enabled && len(res.Hotkeys) == 0:
		line = widgets.Note("No keys saved yet. Type one below and press Enter.", fd.StatusInfo)
	case !res.Enabled:
		line = widgets.Note(fmt.Sprintf("Stock keys: %d saved key(s) are off and the desktop has its own back.", len(res.Hotkeys)), fd.StatusInfo)
	case bound == len(res.Hotkeys):
		line = widgets.Note(fmt.Sprintf("Your keys: %d of %d bound.", bound, len(res.Hotkeys)), fd.StatusGood)
	default:
		line = widgets.Note(fmt.Sprintf("Your keys: %d of %d bound; a key that is not may be held by another shortcut.", bound, len(res.Hotkeys)), fd.StatusWarn)
	}
	mine := widget.NewButtonWithIcon("Use my keys", theme.ConfirmIcon(), func() {
		u.hotkeysOp("Binding your keys...", func(ctx context.Context) (core.HotkeysResult, error) {
			return core.UseMyKeys(ctx, core.HotkeysRequest{Request: u.request()})
		})
	})
	stock := widget.NewButtonWithIcon("Restore stock keys", theme.ContentUndoIcon(), func() {
		u.hotkeysOp("Giving the keys back...", func(ctx context.Context) (core.HotkeysResult, error) {
			return core.RestoreStockKeys(ctx, core.HotkeysRequest{Request: u.request()})
		})
	})
	rows := []fyne.CanvasObject{line,
		container.NewHBox(
			widgets.WithTip(mine, "Every saved key bound, and the volume keys taken. What each key replaces is recorded."),
			widgets.WithTip(stock, "Every key unbound and given back to what held it, and the volume keys given back. "+
				"Your keys stay saved for the other button."),
		),
	}
	for _, k := range res.Adopted {
		rows = append(rows, widgets.Note("Found "+k+" bound on the desktop and added it to your keys.", fd.StatusInfo))
	}
	for _, e := range res.Foreign {
		rows = append(rows, widgets.Note(e+" runs something ototo does not know; it is left as it is.", fd.StatusWarn))
	}
	return widgets.Card("Your keys and the desktop's", rows...)
}

func pair(a, b fyne.CanvasObject) []fyne.CanvasObject { return []fyne.CanvasObject{a, b} }

// hotkeysPage is the section without kglobalaccel: the commands to bind by
// hand, with the person's device ids filled in.
func (u *ui) hotkeysPage() fyne.CanvasObject {
	var b strings.Builder
	b.WriteString("```\n")
	for _, d := range u.status.Devices {
		fmt.Fprintf(&b, "ototo --connect %q    # %s\n", d.ID, devices.Plain(d.Name))
	}
	b.WriteString("ototo --vol-up\nototo --vol-down\n```\n")
	commands := widget.NewRichTextFromMarkdown(b.String())
	commands.Wrapping = fyne.TextWrapOff
	return container.NewVBox(
		widgets.Note("This desktop is not KDE Plasma, or its global shortcut service is not running, so ototo cannot "+
			"bind keys for you.", fd.StatusWarn),
		widgets.Card("Bind these commands yourself",
			widgets.DimWrapped("Every desktop has a place for custom shortcuts, under keyboard settings. Give each one "+
				"a key and one of these commands. A device id is what --connect takes; a name from Outputs works too. "+
				"The two volume commands step the volume and show the indicator."),
			commands,
		),
	)
}

func (u *ui) setHotkey(h config.Hotkey) {
	u.hotkeysOp("Saving the key...", func(ctx context.Context) (core.HotkeysResult, error) {
		return core.SetHotkey(ctx, core.SetHotkeyRequest{Request: u.request(), Hotkey: h})
	})
}

// hotkeysOp runs one operation and redraws the section with its state,
// whether or not it failed: a failed bind still changed the settings.
func (u *ui) hotkeysOp(what string, op func(context.Context) (core.HotkeysResult, error)) {
	u.sh.Perform(what, func(ctx context.Context) error {
		res, err := op(ctx)
		fyne.Do(func() {
			u.hotkeys, u.hotkeysOK = res, true
			u.sh.Refresh()
		})
		return err
	})
}
