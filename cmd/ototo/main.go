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

	"github.com/ushineko/ototo/internal/buildinfo"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/gui"
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
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *version {
		fmt.Printf("ototo %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return 0
	}

	if *status {
		res, err := core.Status(context.Background(), core.StatusRequest{Request: core.Request{
			ConfigPath: *configPath,
			Server:     *server,
			Events: core.Events{Log: func(level core.Level, msg string) {
				if level >= core.LevelWarn {
					fmt.Fprintf(os.Stderr, "ototo: %s: %s\n", level, msg)
				}
			}},
		}})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ototo:", err)
			return 1
		}
		printStatus(os.Stdout, res)
		return 0
	}

	gui.Run(gui.Options{
		Version:    buildinfo.Version,
		Commit:     buildinfo.Commit,
		ConfigPath: *configPath,
		Server:     *server,
		Section:    *section,
		Scheme:     *scheme,
	})
	return 0
}
