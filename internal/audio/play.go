package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/jfreymuth/pulse"
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

// Chime is the built-in sound: two rising tones, a quarter of a second,
// with fades so nothing clicks.
func Chime() Sound {
	const rate = 48000
	tone := func(hz float64, d time.Duration) []float32 {
		n := int(float64(rate) * d.Seconds())
		out := make([]float32, n)
		fade := rate / 100 // 10 ms
		for i := range out {
			v := math.Sin(2 * math.Pi * hz * float64(i) / rate)
			env := 1.0
			if i < fade {
				env = float64(i) / float64(fade)
			} else if n-i < fade {
				env = float64(n-i) / float64(fade)
			}
			out[i] = float32(v * env * 0.35)
		}
		return out
	}
	s := append(tone(660, 110*time.Millisecond), tone(880, 140*time.Millisecond)...)
	return Sound{Rate: rate, Channels: 1, Samples: s}
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

	pos := 0
	reader := pulse.Float32Reader(func(out []float32) (int, error) {
		if pos >= len(s.Samples) {
			return 0, io.EOF
		}
		n := copy(out, s.Samples[pos:])
		pos += n
		return n, nil
	})
	popts := []pulse.PlaybackOption{
		pulse.PlaybackSampleRate(s.Rate),
		pulse.PlaybackMediaName("ototo switch sound"),
		pulse.PlaybackLatency(0.05),
	}
	if s.Channels == 2 {
		popts = append(popts, pulse.PlaybackStereo)
	}
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
	stream.Drain()
	// The reader's EOF is how the clip ends; the library keeps it as the
	// stream's error.
	if err := stream.Error(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("play the sound: %w", err)
	}
	return nil
}
