// Package session owns one live browser: it launches it, performs actions on
// it, captures the page state the agent perceives, and closes it. It is the
// only package in this module that talks to Playwright.
//
// A *Browser is created by Open and must be released with Close; whoever
// calls Open owns the lifecycle. The fine-grained functions (Perform,
// Snapshot, Tabs, Navigate) never start or stop a browser.
//
// There is no package-level state: every Open starts its own Playwright
// driver and browser, so separate Browsers are fully independent and can be
// used from separate goroutines. A single *Browser must be used by one
// goroutine at a time.
package session

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// Size is a width and height in CSS pixels.
type Size struct {
	Width  int
	Height int
}

// DefaultViewport returns the viewport used when Opts.Viewport is nil:
// 1280x720.
func DefaultViewport() Size {
	return Size{Width: 1280, Height: 720}
}

// Opts configures Open. Paths must be absolute; the root package's
// ResolveDirs produces them.
type Opts struct {
	// Browser to launch. The zero value means engine.Default (Chromium).
	Browser engine.Browser
	// Headed shows the browser window. The default is headless.
	Headed bool
	// Viewport is the page size, which is also the size of the recorded
	// video. nil means DefaultViewport().
	Viewport *Size
	// UserDataDir is a browser profile directory, for a persistent context.
	// nil launches an anonymous, non-persistent context with no profile
	// directory at all.
	UserDataDir *string
	// VideoDir receives Playwright's recording of the browser context, which
	// every session makes. Required.
	VideoDir string
	// ArtifactsDir is where the artifact actions (screenshot,
	// createArtifact, readArtifact, listArtifacts) operate. Required; it is
	// created on the first artifact written.
	ArtifactsDir string
	// ConsoleLog is the absolute path of a JSON Lines file that receives,
	// as they happen, every console message, uncaught page error and page
	// crash, from every tab (including tabs the page opens by itself) and
	// worker, plus tab, navigation and active-tab events, and the turn
	// markers LogTurn writes. Each record is written straight to the file,
	// so it survives the process being killed. "" keeps no log. It is
	// best-effort: failing to create or write it never fails the session.
	// The text is recorded as the page logged it, secrets included.
	ConsoleLog string
	// ConsoleLogFieldCap caps each console text, error name, message and
	// stack, and each URL (page, source, worker), in bytes; a record with a
	// cut field is flagged "truncated". 0 means
	// DefaultConsoleLogFieldCap.
	ConsoleLogFieldCap int
	// ConsoleLogMaxBytes caps the console log file. Past it, one "dropped"
	// record is written and the rest are only counted, in the final
	// "closed" record. 0 means DefaultConsoleLogMaxBytes.
	ConsoleLogMaxBytes int64
}

// Browser is one live browser session. Its fields are private: use the
// functions in this package.
type Browser struct {
	engine       engine.Browser
	pw           *playwright.Playwright
	browser      playwright.Browser // nil for a persistent context
	context      playwright.BrowserContext
	active       playwright.Page
	artifactsDir string
	closed       bool
	announcer    *announcer  // nil unless StartAnnouncing was called
	consoleLog   *consoleLog // nil unless Opts.ConsoleLog is set (and could be opened)
}

// Engine reports which browser b is.
func Engine(b *Browser) engine.Browser {
	return b.engine
}

// Open launches a browser as opts describes and returns it with one blank tab
// active. The Playwright driver and the browser must already be installed
// (see the browserinstall package). On error, anything already started is
// shut down again.
func Open(opts Opts) (*Browser, error) {
	eng, err := engine.Resolve(opts.Browser)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	if err := validateOpts(opts); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	size := viewport(opts.Viewport)

	// An explicit, empty RunOptions keeps playwright-go quiet and leaves the
	// caller's global log output alone.
	pw, err := playwright.Run(&playwright.RunOptions{})
	if err != nil {
		return nil, fmt.Errorf("session: starting playwright (call browserinstall.Ensure first?): %w", err)
	}
	b := &Browser{engine: eng, pw: pw, artifactsDir: opts.ArtifactsDir}
	b.consoleLog = startConsoleLog(opts, eng)

	if err := launch(b, browserType(pw, eng), opts, size); err != nil {
		_ = Close(b)
		return nil, fmt.Errorf("session: launching %s: %w", eng, err)
	}
	if b.consoleLog != nil {
		// Before any page loads, so the first document's messages are kept.
		listenConsole(b.consoleLog, b.context)
	}

	// A persistent context opens with a page already; reuse it.
	if pages := b.context.Pages(); len(pages) > 0 {
		setActive(b, pages[0])
		return b, nil
	}
	page, err := b.context.NewPage()
	if err != nil {
		_ = Close(b)
		return nil, fmt.Errorf("session: opening first tab: %w", err)
	}
	setActive(b, page)
	return b, nil
}

func validateOpts(opts Opts) error {
	var errs []error
	if opts.VideoDir == "" || !filepath.IsAbs(opts.VideoDir) {
		errs = append(errs, fmt.Errorf("VideoDir %q must be an absolute path", opts.VideoDir))
	}
	if opts.ArtifactsDir == "" || !filepath.IsAbs(opts.ArtifactsDir) {
		errs = append(errs, fmt.Errorf("ArtifactsDir %q must be an absolute path", opts.ArtifactsDir))
	}
	if opts.UserDataDir != nil && !filepath.IsAbs(*opts.UserDataDir) {
		errs = append(errs, fmt.Errorf("UserDataDir %q must be an absolute path", *opts.UserDataDir))
	}
	if v := opts.Viewport; v != nil && (v.Width <= 0 || v.Height <= 0) {
		errs = append(errs, fmt.Errorf("Viewport %dx%d must be positive", v.Width, v.Height))
	}
	errs = append(errs, validateConsoleLog(opts)...)
	return errors.Join(errs...)
}

func viewport(v *Size) *playwright.Size {
	s := DefaultViewport()
	if v != nil {
		s = *v
	}
	return &playwright.Size{Width: s.Width, Height: s.Height}
}

func browserType(pw *playwright.Playwright, eng engine.Browser) playwright.BrowserType {
	if eng == engine.Firefox {
		return pw.Firefox
	}
	return pw.Chromium
}

// launch starts either a persistent context (with a profile directory) or a
// browser plus a fresh, non-persistent context (with no profile directory).
func launch(b *Browser, bt playwright.BrowserType, opts Opts, size *playwright.Size) error {
	headless := playwright.Bool(!opts.Headed)
	video := &playwright.RecordVideo{Dir: playwright.String(opts.VideoDir), Size: size}

	if opts.UserDataDir != nil {
		ctx, err := bt.LaunchPersistentContext(*opts.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
			Headless:    headless,
			Viewport:    size,
			RecordVideo: video,
		})
		if err != nil {
			return err
		}
		b.context = ctx
		return nil
	}

	br, err := bt.Launch(playwright.BrowserTypeLaunchOptions{Headless: headless})
	if err != nil {
		return err
	}
	b.browser = br
	ctx, err := br.NewContext(playwright.BrowserNewContextOptions{
		Viewport:    size,
		RecordVideo: video,
	})
	if err != nil {
		return err
	}
	b.context = ctx
	return nil
}

// Close shuts b down: the context first (which finishes writing the video),
// then the browser, then the Playwright driver. The console log, if any, is
// finished once the context has closed, so what pages log while unloading is
// kept. Every step runs even if an earlier one fails; the first error is
// returned. Closing an already closed Browser does nothing.
func Close(b *Browser) error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	stopAnnouncer(b)

	var errs []error
	if b.context != nil {
		errs = append(errs, b.context.Close())
	}
	stopConsoleLog(b.consoleLog)
	if b.browser != nil {
		errs = append(errs, b.browser.Close())
	}
	if b.pw != nil {
		errs = append(errs, b.pw.Stop())
	}
	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("session: closing: %w", err)
		}
	}
	return nil
}

// Navigate points the active tab at url. It is for the caller (e.g. the
// starting page of a run), so unlike the agent's goTo action it accepts any
// URL the browser does.
func Navigate(b *Browser, url string) error {
	if err := checkOpen(b); err != nil {
		return err
	}
	if _, err := b.active.Goto(url); err != nil {
		return fmt.Errorf("session: navigating to %q: %w", url, err)
	}
	return nil
}

func checkOpen(b *Browser) error {
	if b == nil || b.closed {
		return errors.New("session: browser is closed")
	}
	return nil
}
