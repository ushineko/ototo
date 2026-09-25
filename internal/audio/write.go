package audio

import (
	"fmt"

	"github.com/jfreymuth/pulse/proto"
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
}

// SinkInputs lists every playing stream.
func (c *Client) SinkInputs() ([]SinkInput, error) {
	var r proto.GetSinkInputInfoListReply
	if err := c.c.Request(&proto.GetSinkInputInfoList{}, &r); err != nil {
		return nil, fmt.Errorf("list the playing streams: %w", err)
	}
	out := make([]SinkInput, 0, len(r))
	for _, s := range r {
		out = append(out, SinkInput{Index: s.SinkInputIndex, Name: s.MediaName, Sink: s.SinkIndex})
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
		req := &proto.MoveSinkInput{SinkInputIndex: in.Index, DeviceIndex: proto.Undefined, DeviceName: sink}
		if err := c.c.Request(req, nil); err != nil {
			refused++
			continue
		}
		moved++
	}
	return moved, refused, nil
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
