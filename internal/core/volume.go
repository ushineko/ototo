package core

import (
	"context"

	"github.com/ushineko/ototo/internal/devices"
)

// VolumeRequest changes the volume of the output that is playing.
type VolumeRequest struct {
	Request
	// Delta is the change in percentage points; 0 reads without changing.
	Delta int
}

// VolumeResult is the output's volume afterwards.
type VolumeResult struct {
	// Sink is the hardware sink the volume belongs to: the default, or the
	// sink JamesDSP plays into when the default is JamesDSP (R6.3).
	Sink    string
	Percent int
	Muted   bool
}

// Volume reads or changes the volume of the sink that is actually playing.
func (s *Switcher) Volume(ctx context.Context, req VolumeRequest) (VolumeResult, error) {
	srv, err := s.connect(req.Server)
	if err != nil {
		return VolumeResult{}, err
	}
	defer func() { _ = srv.Close() }()

	sink, err := s.playingSink(ctx, srv)
	if err != nil {
		return VolumeResult{}, err
	}
	res := VolumeResult{Sink: sink}
	if req.Delta != 0 {
		if _, err := srv.AdjustVolume(sink, req.Delta); err != nil {
			return res, err
		}
	}
	res.Percent, res.Muted, err = srv.Volume(sink)
	return res, err
}

// SetVolumeRequest sets the playing output's volume, or mutes it.
type SetVolumeRequest struct {
	Request
	// Percent is the level to set, 0..150; ignored when Mute is set.
	Percent int
	// Mute, when non-nil, mutes or unmutes instead of setting a level.
	Mute *bool
}

// SetVolume writes to the sink that is actually playing (R6.3).
func (s *Switcher) SetVolume(ctx context.Context, req SetVolumeRequest) (VolumeResult, error) {
	srv, err := s.connect(req.Server)
	if err != nil {
		return VolumeResult{}, err
	}
	defer func() { _ = srv.Close() }()
	sink, err := s.playingSink(ctx, srv)
	if err != nil {
		return VolumeResult{}, err
	}
	res := VolumeResult{Sink: sink}
	if req.Mute != nil {
		if err := srv.SetMute(sink, *req.Mute); err != nil {
			return res, err
		}
	} else if _, err := srv.SetVolume(sink, req.Percent); err != nil {
		return res, err
	}
	res.Percent, res.Muted, err = srv.Volume(sink)
	return res, err
}

// playingSink is the hardware sink the volume belongs to: the default, or
// the sink JamesDSP plays into when the default is JamesDSP.
func (s *Switcher) playingSink(ctx context.Context, srv server) (string, error) {
	info, err := srv.Server()
	if err != nil {
		return "", err
	}
	sink := info.DefaultSink
	if sink == devices.JamesDSPSink {
		if target, err := s.graph().Target(ctx); err == nil && target != "" {
			sink = target
		}
	}
	return sink, nil
}
