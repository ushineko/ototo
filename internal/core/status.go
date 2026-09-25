package core

import (
	"context"
	"errors"
	"os"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/buildinfo"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/graph"
)

// StatusRequest asks what this machine has.
type StatusRequest struct {
	Request
}

// StatusResult is what this machine has: the settings, the sound server and
// the outputs it can see.
type StatusResult struct {
	Version    string
	Commit     string
	ConfigPath string
	// ConfigExists is false on a machine that has never saved settings; the
	// defaults are in force and nothing is wrong.
	ConfigExists bool
	Config       config.Config

	// Server is the sound server, when one answered; ServerError is why not.
	Server      audio.Server
	ServerError string
	// Playing is the hardware sink that is playing: the default, or the
	// sink JamesDSP plays into when the default is JamesDSP (R6.3). The
	// device with this sink is the one marked Default in Devices.
	Playing string
	// Devices is the list as the window shows it (spec R4): the priority
	// order first, remembered devices holding their place, then the rest.
	// Without a server it holds only what the settings remember.
	Devices []devices.Device
	Inputs  int
	// Sources are the inputs by name and description, for the microphone
	// editor (R10.2).
	Sources []Source
	// Headset is what headsetcontrol reported (R9.2), for the Headset card.
	Headset devices.Headset
	// HeadsetTool says whether headsetcontrol is available at all, which is
	// the difference between "off" and "cannot tell".
	HeadsetTool bool
}

// Source is one input.
type Source struct {
	Name        string
	Description string
}

// Status reports the settings, the sound server and the outputs.
//
// A machine with no sound server is a machine to report on, not to fail on:
// the result carries the reason and the settings still.
func Status(ctx context.Context, req StatusRequest) (StatusResult, error) {
	res := StatusResult{Version: buildinfo.Version, Commit: buildinfo.Commit}

	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return res, err
	}
	res.ConfigPath = path
	res.Config = cfg
	if _, statErr := os.Stat(path); statErr == nil {
		res.ConfigExists = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		req.Events.logf(LevelWarn, "settings file %s: %v", path, statErr)
	}

	probes := req.probes()
	in := devices.Inputs{
		Priority:  cfg.DevicePriority,
		Bluetooth: probes.bluetooth(ctx),
		Headset:   probes.headset(ctx),
	}
	res.Headset = in.Headset
	res.HeadsetTool = probes.Headset != nil

	client, err := audio.Connect(req.Server)
	if err != nil {
		res.ServerError = err.Error()
		res.Devices = devices.List(in)
		req.Events.logf(LevelWarn, "%v", err)
		return res, nil
	}
	defer func() { _ = client.Close() }()

	srv, err := client.Server()
	if err != nil {
		res.ServerError = err.Error()
		res.Devices = devices.List(in)
		return res, nil
	}
	res.Server = srv
	res.Playing = srv.DefaultSink
	if srv.DefaultSink == devices.JamesDSPSink {
		if target, err := graph.New().Target(ctx); err == nil && target != "" {
			res.Playing = target
		}
	}
	in.DefaultSink = res.Playing

	sinks, err := client.Sinks()
	if err != nil {
		res.ServerError = err.Error()
		res.Devices = devices.List(in)
		return res, nil
	}
	in.Sinks = sinks
	res.Devices = devices.List(in)

	sources, err := client.Sources()
	if err != nil {
		res.ServerError = err.Error()
		return res, nil
	}
	res.Inputs = len(sources)
	for _, src := range sources {
		res.Sources = append(res.Sources, Source{Name: src.Name, Description: devices.DisplayName(src, nil)})
	}
	req.Events.logf(LevelDebug, "%d outputs, %d inputs from %s %s", len(sinks), len(sources), srv.Name, srv.Version)
	return res, nil
}
