package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

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
	require.NoError(t, Play("", Chime()))
	require.NoError(t, Play(DemoPrefix+"nothing", Chime()), "a demo server must play nothing and succeed")
}
