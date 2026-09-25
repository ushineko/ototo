package instance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestASecondInstanceIsToldToAskTheFirst: the lock is the decision, and a
// request round-trips one line each way.
func TestASecondInstanceIsToldToAskTheFirst(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	_, ok := Ask("show")
	require.False(t, ok, "something answered before anything listened")

	first, err := Listen(func(req string) string { return "got " + req })
	require.NoError(t, err)
	defer first.Close()

	_, err = Listen(func(string) string { return "" })
	require.ErrorIs(t, err, ErrRunning)

	reply, ok := Ask("vol-up")
	require.True(t, ok)
	require.Equal(t, "got vol-up", reply)

	first.Close()
	_, ok = Ask("show")
	require.False(t, ok, "the socket answered after Close")
	again, err := Listen(func(string) string { return "" })
	require.NoError(t, err, "the lock was not released by Close")
	again.Close()
}
