/*
Command ototo is the audio output switcher: a tray-resident window that keeps
the sound on the output you want.

One binary, because the program is the window. It lives in the tray, and the
auto-switch loop, the volume indicator and the notifications all run there. The
flags below exist for two other callers: a desktop shortcut, which needs a
command to run when a key is pressed, and a person reporting a bug, who needs
the machine's state in text. Both are served by this binary without a second
window: a hotkey flag is forwarded to the running instance, and --status prints
and exits.

The flags are parsed with the standard library rather than a command framework:
there are few of them, and three exist so a capture script can deep-link into a
section.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/buildinfo"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/desktop"
	"github.com/ushineko/ototo/internal/gui"
	"github.com/ushineko/ototo/internal/instance"
	"github.com/ushineko/ototo/internal/notify"
)

func main() { os.Exit(run()) }

func run() int {
	section := flag.String("section", "",
		"open on this section: "+strings.Join(gui.SectionNames(), ", "))
	scheme := flag.String("scheme", "",
		"use this colour scheme for this run without saving it: "+strings.Join(gui.SchemeNames(), ", "))
	configPath := flag.String("config", "",
		"settings file (default $XDG_CONFIG_HOME/ototo/config.json)")
	server := flag.String("server", "",
		"sound server (default $PULSE_SERVER, else the session's socket)")
	status := flag.Bool("status", false,
		"print the sound server, the default output and the settings, then exit")
	connect := flag.String("connect", "",
		"switch to this device (a priority id, a sink name, or part of a name) and exit")
	volUp := flag.Bool("vol-up", false, "turn the playing output up one step and exit")
	volDown := flag.Bool("vol-down", false, "turn the playing output down one step and exit")
	desktopStep := flag.String("desktop", "",
		"install or uninstall the desktop steps (autostart, the indicator's window rule, the volume keys), "+
			"or bind KEY ARGS... / unbind KEY for a hotkey of your own, and exit")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	base := core.Request{
		ConfigPath: *configPath,
		Server:     *server,
		Events: core.Events{Log: func(level core.Level, msg string) {
			if level >= core.LevelWarn {
				fmt.Fprintf(os.Stderr, "ototo: %s: %s\n", level, msg)
			}
		}},
	}

	if *version {
		fmt.Println("ototo " + buildinfo.Version)
		return 0
	}

	if *status {
		res, err := core.Status(context.Background(), core.StatusRequest{Request: base})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ototo:", err)
			return 1
		}
		printStatus(os.Stdout, res)
		return 0
	}

	if *connect != "" || *volUp || *volDown {
		return oneShot(base, *connect, *volDown)
	}
	if *desktopStep != "" {
		return desktopFlag(base, *desktopStep, flag.Args())
	}

	return gui.Run(gui.Options{
		Version:    buildinfo.Version,
		Commit:     buildinfo.Commit,
		ConfigPath: *configPath,
		Server:     *server,
		Section:    *section,
		Scheme:     *scheme,
	})
}

/*
oneShot is the hotkey path. When ototo is running, the request goes to it,
so the indicator it draws and the switching state it holds are the ones
that act (D10). Otherwise a fresh Switcher does the work here, as the
original's headless mode did, with the desktop's notifier so a failure
reaches the person who pressed the key. Exit 0 when the change was made, 1
when it was not.
*/
func oneShot(base core.Request, connect string, volDown bool) int {
	request := "vol-up"
	switch {
	case connect != "":
		request = "connect " + connect
	case volDown:
		request = "vol-down"
	}
	if reply, ok := instance.Ask(request); ok {
		status, text, _ := strings.Cut(reply, " ")
		if status == "ok" {
			fmt.Println(text)
			return 0
		}
		fmt.Fprintln(os.Stderr, "ototo:", text)
		return 1
	}

	sw := core.NewSwitcher()
	if bus, err := notify.Session(); err == nil {
		sw.Notifier = bus
	} else {
		fmt.Fprintln(os.Stderr, "ototo: notifications unavailable:", err)
	}
	ctx := context.Background()

	if connect != "" {
		res, err := sw.Switch(ctx, core.SwitchRequest{Request: base, Target: connect, Manual: true})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ototo:", err)
			return 1
		}
		how := "directly"
		if res.ViaJamesDSP {
			how = "through JamesDSP"
		}
		fmt.Printf("switched to %s %s\n", res.Device.Name, how)
		if res.MicName != "" {
			fmt.Printf("input follows: %s\n", res.MicName)
		}
		return 0
	}

	delta := audio.VolumeStep
	if volDown {
		delta = -audio.VolumeStep
	}
	res, err := sw.Volume(ctx, core.VolumeRequest{Request: base, Delta: delta})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ototo:", err)
		return 1
	}
	state := fmt.Sprintf("%d%%", res.Percent)
	if res.Muted {
		state = "muted"
	}
	fmt.Printf("%s: %s\n", res.Sink, state)
	return 0
}

// desktopFlag is --desktop install|uninstall: every step at once, for a
// person who wants the whole set without opening Settings.
func desktopFlag(base core.Request, verb string, rest []string) int {
	var on bool
	switch verb {
	case "install":
		on = true
	case "uninstall":
		on = false
	case "bind", "unbind":
		return bindFlag(base, verb, rest)
	default:
		fmt.Fprintln(os.Stderr, "ototo: --desktop takes install, uninstall, bind or unbind, not", verb)
		return 2
	}
	st, err := core.SetDesktop(context.Background(), core.SetDesktopRequest{Request: base, Autostart: &on, IndicatorRule: &on, VolumeKeys: &on})
	fmt.Printf("autostart: %s\nindicator window rule: %s\nvolume keys: %s\n", onOff(st.Autostart), onOff(st.IndicatorRule), onOff(st.VolumeKeys))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ototo:", err)
		return 1
	}
	return 0
}

/*
bindFlag is --desktop bind KEY ARGS... and --desktop unbind KEY: a key of the
person's own that runs ototo with ARGS, such as `bind Meta+A --connect
"AirPods Pro"` or `bind Meta+Num++ --vol-up`. A command shortcut that held
the key is released and named.
*/
func bindFlag(base core.Request, verb string, rest []string) int {
	if len(rest) == 0 {
		fmt.Fprintf(os.Stderr, "ototo: --desktop %s takes a key, such as Meta+A\n", verb)
		return 2
	}
	key := rest[0]
	ctx := context.Background()
	if verb == "unbind" {
		// The key's hotkey in the settings is removed, which unbinds it;
		// a key bound outside the settings is unbound where it is.
		res, err := core.Hotkeys(ctx, core.HotkeysRequest{Request: base})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ototo:", err)
			return 1
		}
		for _, h := range res.Hotkeys {
			if !strings.EqualFold(h.Key, key) {
				continue
			}
			h.Key = ""
			if _, err := core.SetHotkey(ctx, core.SetHotkeyRequest{Request: base, Hotkey: h.Hotkey}); err != nil {
				fmt.Fprintln(os.Stderr, "ototo:", err)
				return 1
			}
			fmt.Printf("%s unbound\n", key)
			return 0
		}
		had, err := desktop.Unbind(ctx, key)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ototo:", err)
			return 1
		}
		if !had {
			fmt.Printf("%s was not bound to ototo\n", key)
			return 0
		}
		fmt.Printf("%s unbound\n", key)
		return 0
	}
	args := rest[1:]
	h, ok := hotkeyOf(key, args)
	if !ok {
		fmt.Fprintln(os.Stderr, "ototo: --desktop bind takes the key and one of --connect DEVICE, --vol-up, --vol-down")
		return 2
	}
	res, err := core.SetHotkey(ctx, core.SetHotkeyRequest{Request: base, Hotkey: h})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ototo:", err)
		return 1
	}
	if !res.Supported {
		fmt.Printf("%s saved; this desktop cannot bind it, see the Hotkeys section for the command to bind\n", key)
		return 0
	}
	if !res.Enabled {
		fmt.Printf("%s saved; your keys are off, turn them on in Hotkeys\n", key)
		return 0
	}
	fmt.Printf("%s runs: ototo %s\n", key, strings.Join(args, " "))
	return 0
}

// hotkeyOf reads bind's arguments into a hotkey: --connect DEVICE,
// --vol-up or --vol-down.
func hotkeyOf(key string, args []string) (config.Hotkey, bool) {
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
