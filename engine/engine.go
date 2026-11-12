// Package engine names the browsers aclio-go-browser can drive. It is a leaf
// package so every other package can share the type; the root package
// re-exports it, so most callers just write browser.Chromium or
// browser.Firefox.
package engine

import (
	"fmt"
	"slices"
)

// Browser is a browser engine. The zero value means Default.
type Browser string

const (
	Chromium Browser = "chromium"
	Firefox  Browser = "firefox"
)

// Default is the browser used when none is chosen.
const Default = Chromium

// All returns every supported browser, Default first. Each call returns a new
// slice.
func All() []Browser {
	return []Browser{Chromium, Firefox}
}

// Resolve returns b, or Default when b is empty, and an error when b is not a
// supported browser.
func Resolve(b Browser) (Browser, error) {
	if b == "" {
		return Default, nil
	}
	if !slices.Contains(All(), b) {
		return "", fmt.Errorf("unsupported browser %q, want one of %v", b, All())
	}
	return b, nil
}
