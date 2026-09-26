package core

import (
	"context"
	"time"

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
		return s.adjustTracked(srv, sink, req.Delta)
	}
	res.Percent, res.Muted, err = srv.Volume(sink)
	return res, err
}

// volWindow is how long a tracked level stays the base for the next press;
// past it, a fresh reading is taken, so a change made elsewhere is picked up.
const volWindow = 3 * time.Second

// adjustTracked steps the volume from the level ototo last aimed at, when a
// press followed another closely, rather than from the sink's own reading,
// which a Bluetooth headset reports back slowly and rounds to its own steps.
// The level set is absolute and clamped to 0..100, and the intended level is
// what the indicator shows, so a run of presses moves smoothly even while the
// device quantizes underneath.
func (s *Switcher) adjustTracked(srv server, sink string, delta int) (VolumeResult, error) {
	res := VolumeResult{Sink: sink}
	s.mu.Lock()
	base := s.volPercent
	fresh := s.volSink == sink && time.Since(s.volAt) < volWindow
	s.mu.Unlock()
	if !fresh {
		cur, _, err := srv.Volume(sink)
		if err != nil {
			return res, err
		}
		base = cur
	}
	target := base + delta
	if target < 0 {
		target = 0
	}
	if target > 100 {
		target = 100
	}
	if _, err := srv.SetVolume(sink, target); err != nil {
		return res, err
	}
	s.mu.Lock()
	s.volSink, s.volPercent, s.volAt = sink, target, time.Now()
	s.mu.Unlock()
	_, res.Muted, _ = srv.Volume(sink)
	res.Percent = target
	return res, nil
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
