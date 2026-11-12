// Package browserinstall downloads what aclio-go-browser needs to drive a
// browser: the Playwright driver (Node.js plus playwright-core) and the
// browsers you ask for. It is opt-in: nothing in this module imports it, and
// nothing is downloaded unless you call Ensure.
//
// Typical use, once before the first run:
//
//	err := browserinstall.Ensure(browserinstall.Opts{}) // Chromium only
//	err := browserinstall.Ensure(browserinstall.Opts{
//	    Browsers: []engine.Browser{engine.Chromium, engine.Firefox},
//	})
//
// Downloads go to Playwright's default locations (the driver under the user
// cache directory, e.g. ~/Library/Caches/ms-playwright-go on macOS or
// ~/.cache/ms-playwright-go on Linux; browsers under ms-playwright next to
// it). Set PLAYWRIGHT_DRIVER_PATH or PLAYWRIGHT_BROWSERS_PATH to move them;
// the browser layer reads the same variables, so both always agree.
package browserinstall

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"sync"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// Opts configures Ensure.
type Opts struct {
	// Browsers to install. Empty means just engine.Default (Chromium).
	// Duplicates are ignored, and an empty entry means engine.Default.
	Browsers []engine.Browser
	// Output receives the download and install progress. nil means
	// os.Stderr; pass io.Discard to silence it.
	Output io.Writer
	// WithSystemDeps also installs the operating-system packages the browsers
	// need (Linux only, typically needs root). Useful on fresh CI images and
	// containers.
	WithSystemDeps bool
}

// mu serialises Ensure: concurrent installs into the same shared cache
// directories would race each other.
var mu sync.Mutex

// Ensure installs the Playwright driver and the requested browsers if they
// are missing, and does nothing (beyond a quick version check) when they are
// already there. An unsupported browser fails before anything is downloaded.
// It is safe to call from several goroutines; calls run one at a time.
func Ensure(opts Opts) error {
	ro, err := runOptions(opts)
	if err != nil {
		return fmt.Errorf("browserinstall: %w", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if err := playwright.Install(ro); err != nil {
		return fmt.Errorf("browserinstall: %w", err)
	}
	return nil
}

// runOptions maps Opts to playwright-go's install options.
func runOptions(opts Opts) (*playwright.RunOptions, error) {
	browsers, err := browserNames(opts.Browsers)
	if err != nil {
		return nil, err
	}
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}
	return &playwright.RunOptions{
		Browsers: browsers,
		WithDeps: opts.WithSystemDeps,
		Stdout:   out,
		Stderr:   out,
		// Without an explicit Logger, playwright-go redirects the standard
		// library's global log package to Stderr, hijacking the caller's
		// logging for the whole process.
		Logger: slog.New(slog.NewTextHandler(out, nil)),
	}, nil
}

// browserNames resolves and de-duplicates the requested browsers, keeping
// their order. It never returns an empty list: playwright-go reads that as
// "install every browser it knows", WebKit included.
func browserNames(requested []engine.Browser) ([]string, error) {
	if len(requested) == 0 {
		requested = []engine.Browser{engine.Default}
	}
	var names []string
	for _, b := range requested {
		resolved, err := engine.Resolve(b)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(names, string(resolved)) {
			names = append(names, string(resolved))
		}
	}
	return names, nil
}
