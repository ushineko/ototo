package notify

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAZeroTimeoutIsTheDesktopsDefaultNotForever: on the wire 0 means
// "never expire", and every "Switch Failed" sent with the zero value stayed
// on screen until dismissed by hand. The zero value must mean the default.
func TestAZeroTimeoutIsTheDesktopsDefaultNotForever(t *testing.T) {
	require.Equal(t, int32(-1), Notification{}.expiry())
	require.Equal(t, int32(2000), Notification{Timeout: 2000}.expiry())
	require.Equal(t, int32(0), Notification{Sticky: true}.expiry())
}
