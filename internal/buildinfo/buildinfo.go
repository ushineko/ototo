/*
Package buildinfo carries what this binary was built from.

It is a package of its own so that the -ldflags path points at program metadata
rather than at some feature package that happens to be convenient; otherwise
"what version am I" becomes a question you ask the audio client.
*/
package buildinfo

// Version and Commit are injected at build time via -ldflags -X. The defaults
// are what an unadorned `go build` or `go test` produces.
var (
	Version = "dev"
	Commit  = "unknown"
)
