package core

import (
	"context"
	"errors"
	"os"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/buildinfo"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
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
	// Devices is the list as the window shows it (spec R4): the priority
	// order first, remembered devices holding their place, then the rest.
	// Without a server it holds only what the settings remember.
	Devices []devices.Device
	Inputs  int
}

// Status reports the settings, the sound server and the outputs.
//
// A machine with no sound server is a machine to report on, not to fail on:
// the result carries the reason and the settings still.
func Status(_ context.Context, req StatusRequest) (StatusResult, error) {
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

	// The Bluetooth cache and the headset arrive with R9; until then the
	// list names a Bluetooth device by its address and treats the headset
	// as off, which is what the original showed with the adapter down.
	in := devices.Inputs{Priority: cfg.DevicePriority}

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
	in.DefaultSink = srv.DefaultSink

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
	req.Events.logf(LevelDebug, "%d outputs, %d inputs from %s %s", len(sinks), len(sources), srv.Name, srv.Version)
	return res, nil
}
