package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTheChimeIsShortAndQuietAndClickFree: a quarter of a second, well below
// full scale, and starting and ending at silence.
func TestTheChimeIsShortAndQuietAndClickFree(t *testing.T) {
	s := Chime()
	require.InDelta(t, 0.25, s.Duration().Seconds(), 0.01)
	require.Equal(t, 1, s.Channels)
	peak := float32(0)
	for _, v := range s.Samples {
		peak = max(peak, float32(math.Abs(float64(v))))
	}
	require.Less(t, peak, float32(0.5))
	require.Equal(t, float32(0), s.Samples[0])
	require.InDelta(t, 0, s.Samples[len(s.Samples)-1], 0.01)
}

func wav(rate int, channels int, samples []int16) []byte {
	var b bytes.Buffer
	data := make([]byte, 2*len(samples))
	for i, v := range samples {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(v))
	}
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(data)))
	b.WriteString("WAVEfmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate*channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

// TestAPCMWAVIsRead: 16-bit PCM in either channel count reads to samples in
// -1..1 at its own rate; anything else is refused by name.
// TestALeadIsSilenceInFrontOfTheClip: the lead adds that much silence,
// per channel, and leaves the clip itself as it was.
func TestALeadIsSilenceInFrontOfTheClip(t *testing.T) {
	s := Chime()
	led := s.WithLead(100 * time.Millisecond)
	require.Equal(t, s.Duration()+100*time.Millisecond, led.Duration())
	n := len(led.Samples) - len(s.Samples)
	for _, v := range led.Samples[:n] {
		require.Zero(t, v)
	}
	require.Equal(t, s.Samples, led.Samples[n:])
	require.Equal(t, s, s.WithLead(0))
}

func TestAPCMWAVIsRead(t *testing.T) {
	s, err := parseWAV(wav(44100, 2, []int16{0, 16384, -32768, 32767}))
	require.NoError(t, err)
	require.Equal(t, 44100, s.Rate)
	require.Equal(t, 2, s.Channels)
	require.InDelta(t, 0.5, s.Samples[1], 0.001)
	require.InDelta(t, -1, s.Samples[2], 0.001)
	_, err = parseWAV([]byte("not a wav at all"))
	require.ErrorIs(t, err, ErrNotWAV)
	_, err = parseWAV(wav(44100, 3, []int16{0}))
	require.ErrorIs(t, err, ErrNotWAV, "three channels are not something this plays")
}

// TestPlayOnTheLiveServer plays the chime once where a server listens: the
// one check that the stream opens and drains. It is audible.
func TestPlayOnTheLiveServer(t *testing.T) {
	skipWithoutServer(t)
	require.NoError(t, Play("", "", Chime()))
	require.NoError(t, Play(DemoPrefix+"nothing", "", Chime()), "a demo server must play nothing and succeed")
}

// TestALongClipPlaysToTheEnd: playback returns no sooner than the clip is
// long. It once returned after about a second whatever the length, the
// server having been asked to drain before the clip was handed over, and
// a chime behind a second of leading silence was never heard.
func TestALongClipPlaysToTheEnd(t *testing.T) {
	skipWithoutServer(t)
	clip := Sound{Rate: 48000, Channels: 1, Samples: make([]float32, 48000*2)} // 2 s of silence
	started := time.Now()
	require.NoError(t, Play("", "", clip))
	require.GreaterOrEqual(t, time.Since(started), 1900*time.Millisecond, "playback returned before the clip ended")
}
