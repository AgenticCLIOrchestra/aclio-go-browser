package session

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// resolveGoTo resolves a goTo target against the current page URL and checks
// the result. action.ValidateURL already limits raw to http(s) or relative
// references; the resolved URL must be http(s) too, so a relative reference
// can't inherit a non-web scheme from the current page (about:blank, file:,
// …).
func resolveGoTo(current, raw string) (string, error) {
	if err := action.ValidateURL(raw); err != nil {
		return "", err
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("url %q: %w", raw, err)
	}
	if ref.IsAbs() {
		return ref.String(), nil
	}
	base, err := url.Parse(current)
	if err != nil {
		return "", fmt.Errorf("resolving %q against the current page %q: %w", raw, current, err)
	}
	resolved := base.ResolveReference(ref)
	switch strings.ToLower(resolved.Scheme) {
	case "http", "https":
		return resolved.String(), nil
	default:
		return "", fmt.Errorf("relative url %q needs an http(s) page to resolve against; the current page is %q", raw, current)
	}
}
