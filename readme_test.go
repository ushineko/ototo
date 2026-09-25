package ototo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/mermaid"
)

// TestEveryDiagramIsRendered: a fence edited without make generate would
// draw as its source in the window, with a caption asking for the render.
func TestEveryDiagramIsRendered(t *testing.T) {
	missing, stale, err := mermaid.Check(".", "diagrams")
	require.NoError(t, err)
	require.Empty(t, missing, "run make generate")
	require.Empty(t, stale, "run make generate")
	require.True(t, mermaid.NewSet(Docs(), "diagrams").Has(routingDiagram(t)))
}

// routingDiagram is the README's first mermaid fence.
func routingDiagram(t *testing.T) string {
	t.Helper()
	_, rest, ok := strings.Cut(README(), "```mermaid\n")
	require.True(t, ok, "the README has no mermaid fence")
	src, _, ok := strings.Cut(rest, "```")
	require.True(t, ok)
	return src
}

// TestTheReadmeIsEmbedded: the window shows this document, so it must be
// the document, version line and all.
func TestTheReadmeIsEmbedded(t *testing.T) {
	require.Contains(t, README(), "**Version**: ")
	require.Contains(t, README(), "## How it routes audio")
}
