// Package browser lets an agentic CLI (Claude Code or Codex, via aclio-go)
// drive a web browser without ever touching it: each turn the agent is shown
// the page and answers with actions (see the action package), which are
// validated against its persona (see the preset package) and performed on its
// behalf.
package browser

import (
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/session"
)

// Browser is a browser engine: Chromium or Firefox. It is engine.Browser,
// re-exported so callers rarely need the engine package.
type Browser = engine.Browser

const (
	Chromium = engine.Chromium
	Firefox  = engine.Firefox
)

// Size is a width and height in CSS pixels. It is session.Size, re-exported.
type Size = session.Size

// DefaultViewport returns the viewport used when Options.Viewport is nil:
// 1280x720.
func DefaultViewport() Size {
	return session.DefaultViewport()
}
