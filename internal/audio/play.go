package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

/*
A sound on a switch (spec 001, "Unreleased" in the changelog): a short
built-in chime, or a WAV file of the person's own, played to the default
output over the sound server. No subprocess: the same library that speaks
the protocol plays a stream.
*/

// Sound is a clip ready to play: samples in -1..1, interleaved.
type Sound struct {
	Rate     int
	Channels int
	Samples  []float32
}

// WithLead is the sound with lead of silence in front of it. Silence in the
// stream, unlike a wait before it, keeps the stream open while the sink
// starts: a Bluetooth sink exists before the headphones render anything,
// and a clip that drained before they did was never heard.
func (s Sound) WithLead(lead time.Duration) Sound {
	if lead <= 0 || s.Rate == 0 {
		return s
	}
	ch := max(s.Channels, 1)
	n := int(lead.Seconds()*float64(s.Rate)) * ch
	out := s
	out.Samples = append(make([]float32, n, n+len(s.Samples)), s.Samples...)
	return out
}

// Chime is the built-in sound: one low boop, a third of a second, a tone
// falling from 170 Hz to 130 Hz with a touch of its octave for body, that
// swells for 15 ms and dies away. It sits below the desktop's own
// notification sounds, which are bright, so it is not mistaken for one.
func Chime() Sound {
	const (
		rate   = 48000
		length = 320 * time.Millisecond
		attack = rate * 15 / 1000
	)
	n := int(rate * length.Seconds())
	out := make([]float32, n)
	phase := 0.0
	for i := range out {
		t := float64(i) / float64(n)
		hz := 170 - 40*t
		phase += 2 * math.Pi * hz / rate
		v := math.Sin(phase) + 0.25*math.Sin(2*phase)
		env := math.Exp(-3.5 * t)
		if i < attack {
			env *= float64(i) / float64(attack)
		}
		if n-i < attack {
			env *= float64(n-i) / float64(attack)
		}
		out[i] = float32(v * env * 0.38)
	}
	return Sound{Rate: rate, Channels: 1, Samples: out}
}

// ErrNotWAV is a file that is not 16-bit PCM WAV, which is the one format
// read here: it is what every sound editor writes, and it needs no decoder.
var ErrNotWAV = errors.New("not a 16-bit PCM WAV file")

// ReadWAV reads a 16-bit PCM WAV file.
func ReadWAV(path string) (Sound, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path the person chose
	if err != nil {
		return Sound{}, fmt.Errorf("read the sound: %w", err)
	}
	return parseWAV(raw)
}

func parseWAV(raw []byte) (Sound, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return Sound{}, ErrNotWAV
	}
	var s Sound
	var bits int
	pos := 12
	for pos+8 <= len(raw) {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(raw) {
			size = len(raw) - body
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return Sound{}, ErrNotWAV
			}
			format := binary.LittleEndian.Uint16(raw[body:])
			s.Channels = int(binary.LittleEndian.Uint16(raw[body+2:]))
			s.Rate = int(binary.LittleEndian.Uint32(raw[body+4:]))
			bits = int(binary.LittleEndian.Uint16(raw[body+14:]))
			if format != 1 || bits != 16 || s.Channels < 1 || s.Channels > 2 || s.Rate <= 0 {
				return Sound{}, ErrNotWAV
			}
		case "data":
			if s.Rate == 0 {
				return Sound{}, ErrNotWAV
			}
			n := size / 2
			s.Samples = make([]float32, n)
			for i := range n {
				v := int16(binary.LittleEndian.Uint16(raw[body+2*i:])) //nolint:gosec // a sample
				s.Samples[i] = float32(v) / 32768
			}
			return s, nil
		}
		pos = body + size + size%2
	}
	return Sound{}, ErrNotWAV
}

// Duration is how long the clip plays.
func (s Sound) Duration() time.Duration {
	if s.Rate == 0 || s.Channels == 0 {
		return 0
	}
	return time.Duration(float64(len(s.Samples)) / float64(s.Channels) / float64(s.Rate) * float64(time.Second))
}

/*
Play plays the clip into sink, or the default output when sink is "", and
returns when it has drained. server is as for Connect; a demo server plays
nothing and returns at once. The stream is named for the desktop's mixer,
so a person who sees it there knows what it is.
*/
func Play(server, sink string, s Sound) error {
	if IsDemo(server) || len(s.Samples) == 0 {
		return nil
	}
	opts := []pulse.ClientOption{pulse.ClientApplicationName("ototo")}
	if server != "" {
		opts = append(opts, pulse.ClientServerString(server))
	}
	c, err := pulse.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("connect to the sound server: %w", err)
	}
	defer c.Close()

	// ended closes when the reader has handed over the last sample. The
	// server is asked to drain only then: asked earlier, it answers once
	// what it holds so far has played, about a second, and closing the
	// stream on that answer drops the rest of the clip. That lost every
	// chime placed after a second of leading silence.
	pos := 0
	ended := make(chan struct{})
	reader := pulse.Float32Reader(func(out []float32) (int, error) {
		if pos >= len(s.Samples) {
			close(ended)
			return 0, pulse.EndOfData
		}
		n := copy(out, s.Samples[pos:])
		pos += n
		return n, nil
	})
	popts := []pulse.PlaybackOption{
		pulse.PlaybackSampleRate(s.Rate),
		pulse.PlaybackMediaName("ototo switch sound"),
		pulse.PlaybackLatency(0.05),
		// A sound event, as the desktop's own notification sounds are.
		pulse.PlaybackRawOption(func(o *proto.CreatePlaybackStream) {
			if o.Properties == nil {
				o.Properties = proto.PropList{}
			}
			o.Properties["media.role"] = proto.PropListString("event")
		}),
	}
	if s.Channels == 2 {
		popts = append(popts, pulse.PlaybackStereo)
	}
	// The sink is a request: WirePlumber remembers a target per application
	// name and moves the stream there when it has one, so the sound may
	// still ride through the default route. The route is confirmed before
	// the sound is played, so either lands on the new device.
	if sink != "" {
		target, err := c.SinkByID(sink)
		if err != nil {
			return fmt.Errorf("find the sink %s: %w", sink, err)
		}
		popts = append(popts, pulse.PlaybackSink(target))
	}
	stream, err := c.NewPlayback(reader, popts...)
	if err != nil {
		return fmt.Errorf("open a playback stream: %w", err)
	}
	defer stream.Close()
	stream.Start()
	select {
	case <-ended:
	case <-time.After(s.Duration() + drainGrace):
		return fmt.Errorf("play the sound: the server stopped asking for it after %s", s.Duration()+drainGrace)
	}
	stream.Drain()
	if err := stream.Error(); err != nil {
		return fmt.Errorf("play the sound: %w", err)
	}
	return nil
}

// drainGrace is how much longer than the clip the server may take to ask
// for all of it before playback is given up.
const drainGrace = 5 * time.Second
