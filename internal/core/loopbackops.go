package core

import (
	"context"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/loopback"
)

// LoopbackRequest asks for the loopback's state.
type LoopbackRequest struct {
	Request
}

// LoopbackState reports the line-in loopback (R9.3).
func (s *Switcher) LoopbackState(ctx context.Context, req LoopbackRequest) (loopback.State, error) {
	srv, err := s.connect(req.Server)
	if err != nil {
		return loopback.State{}, err
	}
	defer func() { _ = srv.Close() }()
	sources, err := srv.Sources()
	if err != nil {
		return loopback.State{}, err
	}
	return s.loopback().State(ctx, loopback.LineInSource(sources)), nil
}

// SetLoopbackRequest turns the loopback on or off.
type SetLoopbackRequest struct {
	Request
	Enabled bool
}

// SetLoopback turns the loopback on or off and remembers the choice, so
// the resident process restores a direct loopback at its next start.
func (s *Switcher) SetLoopback(ctx context.Context, req SetLoopbackRequest) (loopback.State, error) {
	srv, err := s.connect(req.Server)
	if err != nil {
		return loopback.State{}, err
	}
	defer func() { _ = srv.Close() }()
	sources, err := srv.Sources()
	if err != nil {
		return loopback.State{}, err
	}
	sinks, err := srv.Sinks()
	if err != nil {
		return loopback.State{}, err
	}
	st, err := s.loopback().Set(ctx, req.Enabled, loopback.LineInSource(sources), loopback.TargetSink(sinks))
	if err != nil {
		return st, err
	}
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return st, err
	}
	cfg.LoopbackEnabled = req.Enabled
	if err := config.Save(path, cfg); err != nil {
		return st, err
	}
	req.Events.logf(LevelInfo, "line-in loopback %s (%s)", onOff(req.Enabled), st.Mode)
	return st, nil
}

/*
RestoreLoopback is the start-of-process step (R9.3): when the settings say
the loopback was on and no systemd unit manages it, start the direct child
again. With the unit, systemd is the one that remembers.
*/
func (s *Switcher) RestoreLoopback(ctx context.Context, req LoopbackRequest) (loopback.State, error) {
	cfg, _, err := config.Load(req.ConfigPath)
	if err != nil {
		return loopback.State{}, err
	}
	if !cfg.LoopbackEnabled {
		return loopback.State{}, nil
	}
	srv, err := s.connect(req.Server)
	if err != nil {
		return loopback.State{}, err
	}
	defer func() { _ = srv.Close() }()
	sources, err := srv.Sources()
	if err != nil {
		return loopback.State{}, err
	}
	source := loopback.LineInSource(sources)
	if source == "" || s.loopback().ServiceInstalled(ctx) {
		return s.loopback().State(ctx, source), nil
	}
	sinks, err := srv.Sinks()
	if err != nil {
		return loopback.State{}, err
	}
	req.Events.logf(LevelInfo, "restoring the line-in loopback for %s", source)
	return s.loopback().Set(ctx, true, source, loopback.TargetSink(sinks))
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
