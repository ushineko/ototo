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

	info, err := srv.Server()
	if err != nil {
		return VolumeResult{}, err
	}
	sink := info.DefaultSink
	if sink == devices.JamesDSPSink {
		if target, err := s.graph().Target(ctx); err == nil && target != "" {
			sink = target
		}
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
