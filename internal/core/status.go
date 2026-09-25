package core

import (
	"context"
	"errors"
	"os"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/buildinfo"
	"github.com/ushineko/ototo/internal/config"
)

// StatusRequest asks what this machine has.
type StatusRequest struct {
	Request
}

// Output is one sink as the status reports it.
type Output struct {
	Name        string
	Description string
	Default     bool
	Connected   bool
	Mute        bool
	Volume      int
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
	Outputs     []Output
	Inputs      int
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

	client, err := audio.Connect(req.Server)
	if err != nil {
		res.ServerError = err.Error()
		req.Events.logf(LevelWarn, "%v", err)
		return res, nil
	}
	defer func() { _ = client.Close() }()

	srv, err := client.Server()
	if err != nil {
		res.ServerError = err.Error()
		return res, nil
	}
	res.Server = srv

	sinks, err := client.Sinks()
	if err != nil {
		res.ServerError = err.Error()
		return res, nil
	}
	for _, s := range sinks {
		res.Outputs = append(res.Outputs, Output{
			Name:        s.Name,
			Description: s.Description,
			Default:     s.Name == srv.DefaultSink,
			Connected:   s.Connected(),
			Mute:        s.Mute,
			Volume:      s.VolumePercent,
		})
	}
	sources, err := client.Sources()
	if err != nil {
		res.ServerError = err.Error()
		return res, nil
	}
	res.Inputs = len(sources)
	req.Events.logf(LevelDebug, "%d outputs, %d inputs from %s %s", len(sinks), len(sources), srv.Name, srv.Version)
	return res, nil
}
