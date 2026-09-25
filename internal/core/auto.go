package core

import (
	"context"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
)

// AutoSwitchRequest is one tick of the auto-switch (R5.3).
type AutoSwitchRequest struct {
	Request
}

// AutoSwitchResult is what the tick decided.
type AutoSwitchResult struct {
	// Enabled is config.auto_switch; a tick with it off does nothing.
	Enabled bool
	// Target is the device the priority order wants, when there is one.
	Target *devices.Device
	// Switched says a switch ran; Switch is its result. Reason says why, or
	// why not, in words for a log.
	Switched bool
	Switch   *SwitchResult
	Reason   string
}

// snapshot is what the decision is made from.
type snapshot struct {
	list        []devices.Device
	defaultSink string
	// currentValid is the connectedness of the default sink; the JamesDSP
	// sink counts as connected, as it did in the original's sink list.
	currentValid bool
	jdspSink     bool   // the JamesDSP sink exists
	jdspOutputs  bool   // the filter has output ports in the graph
	jdspTarget   string // where those ports play; "" when floating
	jdspBroken   bool
}

// decision is what to do about it.
type decision struct {
	target       *devices.Device
	switchNow    bool
	resetBreaker bool
	reason       string
}

/*
decide is the original's run_auto_switch, as a pure function.

The target is the first device in priority order that has a connected sink.
A switch happens when the default is not the target; when the default is the
target's hardware but JamesDSP is present and working, so the routing should
go through it (enforcement); or when the default is JamesDSP and its output
plays into the wrong sink, or into nothing while it has ports (floating).
When nothing in the priority order is available and the current default
cannot play, the first device that can is the target (fallback).
*/
func decide(sn snapshot) decision {
	var d decision
	for i := range sn.list {
		dev := &sn.list[i]
		if dev.Online && dev.Connected && dev.Sink != devices.JamesDSPSink {
			d.target = dev
			break
		}
	}

	valid := sn.currentValid
	if d.target != nil {
		want := d.target.Sink
		switch {
		case sn.defaultSink != want && sn.defaultSink != devices.JamesDSPSink:
			d.switchNow = true
			d.reason = "the default output is not the first available device"
		case sn.defaultSink == want && sn.jdspSink:
			if sn.jdspBroken && sn.jdspOutputs {
				d.resetBreaker = true
			}
			if (!sn.jdspBroken || d.resetBreaker) && sn.jdspOutputs {
				d.switchNow = true
				d.reason = "JamesDSP is present and the output bypasses it"
			} else {
				d.reason = "the first available device is playing"
			}
		case sn.defaultSink == want:
			d.reason = "the first available device is playing"
		case sn.defaultSink == devices.JamesDSPSink:
			switch {
			case sn.jdspTarget != "" && sn.jdspTarget != want:
				d.switchNow = true
				d.reason = "JamesDSP plays into " + sn.jdspTarget + ", not the first available device"
			case sn.jdspTarget != "":
				valid = true
				d.reason = "JamesDSP plays into the first available device"
			case !sn.jdspOutputs:
				valid = true
				d.reason = "JamesDSP is idle"
			default:
				d.switchNow = true
				d.reason = "JamesDSP is floating"
			}
		}
	}

	if d.target == nil && !valid {
		for i := range sn.list {
			dev := &sn.list[i]
			if dev.Online && dev.Connected {
				d.target = dev
				d.switchNow = true
				d.reason = "nothing in the priority order is available and the current output cannot play"
				break
			}
		}
	}
	if d.target == nil && d.reason == "" {
		d.reason = "no device can play"
	}
	return d
}

// AutoSwitch runs one tick: reads the state, decides, and switches when the
// decision says so. It does nothing when auto_switch is off.
func (s *Switcher) AutoSwitch(ctx context.Context, req AutoSwitchRequest) (AutoSwitchResult, error) {
	cfg, _, err := config.Load(req.ConfigPath)
	if err != nil {
		return AutoSwitchResult{}, err
	}
	res := AutoSwitchResult{Enabled: cfg.AutoSwitch}
	if !cfg.AutoSwitch {
		res.Reason = "auto-switch is off"
		return res, nil
	}
	srv, err := s.connect(req.Server)
	if err != nil {
		return res, err
	}
	defer func() { _ = srv.Close() }()

	sn, err := s.snapshot(ctx, srv, cfg, req.probes(), req.Events)
	if err != nil {
		return res, err
	}
	d := decide(sn)
	target := "none"
	if d.target != nil {
		target = d.target.Name
	}
	req.Events.logf(LevelDebug, "auto-switch: default %s, target %s, switch=%v: %s", sn.defaultSink, target, d.switchNow, d.reason)
	if d.resetBreaker {
		req.Events.logf(LevelInfo, "JamesDSP outputs are back; closing the breaker")
		s.mu.Lock()
		s.jdspBroken = false
		s.mu.Unlock()
	}
	res.Target = d.target
	res.Reason = d.reason
	if !d.switchNow || d.target == nil {
		return res, nil
	}
	req.Events.logf(LevelInfo, "auto-switching to %s: %s", d.target.Name, d.reason)
	sw, err := s.switchTo(ctx, srv, cfg, *d.target, req.Events)
	if err != nil {
		return res, err
	}
	res.Switched = true
	res.Switch = &sw
	return res, nil
}

// snapshot reads what decide needs.
func (s *Switcher) snapshot(ctx context.Context, srv server, cfg config.Config, probes Probes, ev Events) (snapshot, error) {
	info, err := srv.Server()
	if err != nil {
		return snapshot{}, err
	}
	sinks, err := srv.Sinks()
	if err != nil {
		return snapshot{}, err
	}
	sn := snapshot{
		defaultSink: info.DefaultSink,
		jdspBroken:  s.JamesDSPBroken(),
		list: devices.List(devices.Inputs{
			Sinks: sinks, DefaultSink: info.DefaultSink, Priority: cfg.DevicePriority,
			Bluetooth: probes.bluetooth(ctx), Headset: probes.headset(ctx),
		}),
	}
	sn.currentValid, sn.jdspSink = currentValid(sinks, sn.list, info.DefaultSink)
	if sn.jdspSink {
		outs, err := s.graph().JamesDSPOutputs(ctx)
		if err != nil {
			// No pw-link, or PipeWire not answering: the routing cannot be
			// read, so treat JamesDSP as absent and switch the hardware.
			ev.logf(LevelDebug, "reading the PipeWire graph: %v", err)
		}
		sn.jdspOutputs = len(outs) > 0
		if sn.jdspOutputs {
			target, err := s.graph().Target(ctx)
			if err == nil {
				sn.jdspTarget = target
			}
		}
	}
	return sn, nil
}

// currentValid is whether the default sink can play, and whether the
// JamesDSP sink exists.
func currentValid(sinks []audio.Device, list []devices.Device, defaultSink string) (valid, jdsp bool) {
	for _, s := range sinks {
		if s.Name == devices.JamesDSPSink {
			jdsp = true
		}
	}
	if defaultSink == devices.JamesDSPSink {
		return jdsp, jdsp
	}
	for _, d := range list {
		if d.Sink == defaultSink {
			return d.Connected, jdsp
		}
	}
	return false, jdsp
}
