package audio

import (
	"fmt"
	"math"

	"github.com/jfreymuth/pulse/proto"

	"github.com/ushineko/ototo/internal/graph"
)

// Volume limits, as the original clamped them: pactl accepts more, and a
// slider that reaches 150% is loud enough to be a mistake already.
const (
	MaxVolumePercent = 150
	// VolumeStep is what one press of a volume key changes.
	VolumeStep = 5
)

// SetDefaultSink makes name the default output.
func (c *Client) SetDefaultSink(name string) error {
	if err := c.c.Request(&proto.SetDefaultSink{SinkName: name}, nil); err != nil {
		return fmt.Errorf("set the default output to %s: %w", name, err)
	}
	return nil
}

// SetDefaultSource makes name the default input.
func (c *Client) SetDefaultSource(name string) error {
	if err := c.c.Request(&proto.SetDefaultSource{SourceName: name}, nil); err != nil {
		return fmt.Errorf("set the default input to %s: %w", name, err)
	}
	return nil
}

// Volume reads a sink's volume as a percentage and whether it is muted.
func (c *Client) Volume(sink string) (percent int, muted bool, err error) {
	var r proto.GetSinkInfoReply
	if err := c.c.Request(&proto.GetSinkInfo{SinkIndex: proto.Undefined, SinkName: sink}, &r); err != nil {
		return 0, false, fmt.Errorf("read the volume of %s: %w", sink, err)
	}
	return Percent(r.ChannelVolumes), r.Mute, nil
}

// SetVolume sets every channel of a sink to percent, clamped to 0..150.
// It returns the value set.
func (c *Client) SetVolume(sink string, percent int) (int, error) {
	percent = clamp(percent)
	var r proto.GetSinkInfoReply
	if err := c.c.Request(&proto.GetSinkInfo{SinkIndex: proto.Undefined, SinkName: sink}, &r); err != nil {
		return 0, fmt.Errorf("read the channels of %s: %w", sink, err)
	}
	vols := make(proto.ChannelVolumes, len(r.ChannelVolumes))
	for i := range vols {
		vols[i] = fromPercent(percent)
	}
	if err := c.c.Request(&proto.SetSinkVolume{SinkIndex: proto.Undefined, SinkName: sink, ChannelVolumes: vols}, nil); err != nil {
		return 0, fmt.Errorf("set the volume of %s: %w", sink, err)
	}
	return percent, nil
}

// AdjustVolume moves a sink's volume by delta percentage points, clamped,
// and returns the new value. The loudest channel is the reference, so a
// stereo sink whose channels differ ends up level.
func (c *Client) AdjustVolume(sink string, delta int) (int, error) {
	current, _, err := c.Volume(sink)
	if err != nil {
		return 0, err
	}
	return c.SetVolume(sink, current+delta)
}

// SetMute mutes or unmutes a sink.
func (c *Client) SetMute(sink string, mute bool) error {
	if err := c.c.Request(&proto.SetSinkMute{SinkIndex: proto.Undefined, SinkName: sink, Mute: mute}, nil); err != nil {
		return fmt.Errorf("set mute on %s: %w", sink, err)
	}
	return nil
}

// SinkInput is one stream playing into a sink.
type SinkInput struct {
	Index uint32
	Name  string
	Sink  uint32
	Node  string // node.name, which names the Bluetooth bridge's own stream
	// Volume is the stream's own per-channel volume.
	Volume proto.ChannelVolumes
}

// SinkInputs lists every playing stream.
func (c *Client) SinkInputs() ([]SinkInput, error) {
	var r proto.GetSinkInputInfoListReply
	if err := c.c.Request(&proto.GetSinkInputInfoList{}, &r); err != nil {
		return nil, fmt.Errorf("list the playing streams: %w", err)
	}
	out := make([]SinkInput, 0, len(r))
	for _, s := range r {
		in := SinkInput{Index: s.SinkInputIndex, Name: s.MediaName, Sink: s.SinkIndex, Volume: s.ChannelVolumes}
		if node, ok := s.Properties["node.name"]; ok {
			in.Node = node.String()
		}
		out = append(out, in)
	}
	return out, nil
}

/*
MoveSinkInputs moves every playing stream to the named sink and reports how
many moved and how many refused.

A refusal is not a failure of the switch: a stream can be pinned, or gone
between the listing and the move, and the original ignored each one. The
count is returned so the caller can say so.
*/
func (c *Client) MoveSinkInputs(sink string) (moved, refused int, err error) {
	inputs, err := c.SinkInputs()
	if err != nil {
		return 0, 0, err
	}
	for _, in := range inputs {
		if !Movable(in) {
			continue
		}
		req := &proto.MoveSinkInput{SinkInputIndex: in.Index, DeviceIndex: proto.Undefined, DeviceName: sink}
		if err := c.c.Request(req, nil); err != nil {
			refused++
			continue
		}
		moved++
	}
	return moved, refused, nil
}

// Movable is whether a switch moves the stream (spec 006 R4): every stream
// but the Bluetooth bridge's own, which plays into the headset by links the
// graph package makes, and moved into JamesDSP's sink would feed back.
func Movable(in SinkInput) bool { return in.Node != graph.BridgeOut }

/*
SetStreamLevel sets the volume of the stream whose node is named node, every
channel to the same linear gain (1 is unity, 0.5 is -6 dB). A stream already
there is not written, so a repeated call raises no change events, and
changed says whether anything was. No such stream is not an error: there is
nothing to set.
*/
func (c *Client) SetStreamLevel(node string, linear float64) (changed bool, err error) {
	inputs, err := c.SinkInputs()
	if err != nil {
		return false, err
	}
	want := linearVolume(linear)
	for _, in := range inputs {
		if in.Node != node || len(in.Volume) == 0 || allAt(in.Volume, want) {
			continue
		}
		vols := make(proto.ChannelVolumes, len(in.Volume))
		for i := range vols {
			vols[i] = want
		}
		if err := c.c.Request(&proto.SetSinkInputVolume{SinkInputIndex: in.Index, ChannelVolumes: vols}, nil); err != nil {
			return changed, fmt.Errorf("set the level of %s: %w", node, err)
		}
		changed = true
	}
	return changed, nil
}

// linearVolume is the wire value of a linear gain: the protocol's volume is
// cubic, so a gain of 0.5 is the cube root of 0.5 of the norm.
func linearVolume(linear float64) proto.Volume {
	return proto.Volume(math.Round(math.Cbrt(linear) * volumeNorm))
}

func allAt(vols proto.ChannelVolumes, v proto.Volume) bool {
	for _, x := range vols {
		if x != v {
			return false
		}
	}
	return true
}

func clamp(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > MaxVolumePercent {
		return MaxVolumePercent
	}
	return percent
}

func fromPercent(percent int) proto.Volume {
	return proto.Volume(uint64(percent) * volumeNorm / 100) //nolint:gosec // 0..150 fits
}
