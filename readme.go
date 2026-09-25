/*
Package ototo is the repository's root: the README and its diagrams, embedded
so that the window's About section shows the document itself rather than a
second, less careful copy.

The mermaid fences in README.md render to PNG at development time, through
fynedesygn-mermaid and mmdc, into diagrams/; go generate does it, and a test
fails when a fence has no image or an image has no fence.
*/
package ototo

import (
	"embed"
	"io/fs"
)

//go:generate go run github.com/ushineko/fynedesygn/cmd/fynedesygn-mermaid -root . -out diagrams

//go:embed README.md diagrams/*.png
var docs embed.FS

// README is the document.
func README() string {
	b, _ := docs.ReadFile("README.md")
	return string(b)
}

// Docs is the embedded tree: README.md and diagrams/, for the document pane
// to resolve the diagrams from. The screenshots are a page of their own on
// GitHub, linked from the README, and are not embedded.
func Docs() fs.FS { return docs }
