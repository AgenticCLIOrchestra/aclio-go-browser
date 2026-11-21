package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Options configures a run: which browser it drives, where it keeps its files,
// which browser profile it uses and whether the browser window is shown.
// Nothing a run writes is ever deleted by this library: the working directory,
// the artifacts and the recording stay on disk after the run for the caller to
// keep or clean up.
type Options struct {
	// Browser is the browser to drive. The zero value means Chromium.
	Browser Browser
	// WorkingDir holds everything the run needs on disk: the agent's working
	// directory with each turn's screenshot and DOM, debug dumps and
	// Playwright's video recording of the browser context, which every run
	// makes. nil creates a fresh directory under the system temp dir, named
	// aclio-go-browser-{UTC timestamp}-{random}, so listing them
	// alphabetically lists them oldest first. A directory that doesn't exist
	// yet is created.
	WorkingDir *string
	// ArtifactsDir is where the screenshot, createArtifact, readArtifact and
	// listArtifacts actions operate, and the only place the agent's artifacts
	// can be read from: the agent gets no direct read access to it. nil means
	// {WorkingDir}/artifacts. It is created on the first write, so a run that
	// creates no artifacts leaves no directory behind.
	ArtifactsDir *string
	// UserDataDir is a browser user data (profile) directory to run with, so
	// cookies, logins and local storage persist across runs. A profile belongs
	// to one browser: don't share a directory between Chromium and Firefox.
	// nil runs an anonymous session: a fresh, non-persistent browser context
	// with no profile directory anywhere, not even a temporary one.
	UserDataDir *string
	// Headed shows the browser window. The default is headless.
	Headed bool
	// Viewport is the page size in CSS pixels, which is also the size of the
	// recorded video. nil means DefaultViewport() (1280x720).
	Viewport *Size
	// NoConsoleLog turns off the console log Run keeps in the debug dir:
	// {stamp}-{name}-console.jsonl, one JSON record per line, written as it
	// happens, of every console message, uncaught page error and page crash
	// in every tab and worker, with tab, navigation and turn markers (see
	// session.Opts.ConsoleLog). No agent ever sees it, whatever its preset;
	// it is for whoever investigates a run afterwards. It holds whatever
	// the page logs, tokens and personal data included, like the DOM dumps
	// and prompts already in the debug dir. Run keeps it only when there is
	// a debug dir. Callers of Loop set session.Opts.ConsoleLog themselves.
	NoConsoleLog bool
	// NoNetworkLog turns off the network log Run keeps in the debug dir:
	// {stamp}-{name}-network.jsonl, written as it happens, of every response
	// with status 400 or above (with the start of a text body) and every
	// request that got no response, in every tab and worker, with the same
	// tab and turn markers (see session.Opts.NetworkLog). No agent ever sees
	// it. URLs are redacted, only an allowlist of headers is kept and no
	// request body is, but error bodies are kept as the server sent them and
	// may hold personal data. Run keeps it only when there is a debug dir.
	NoNetworkLog bool
	// NetworkLogAll makes the network log also record a one-line summary of
	// every response, and the duration of every request.
	NetworkLogAll bool
}

// Dirs are the absolute directories a run uses, resolved from Options.
type Dirs struct {
	Working   string
	Artifacts string
	// UserData is the browser profile directory, or nil for an anonymous
	// session.
	UserData *string
}

// artifactsSubdir is ArtifactsDir's default, under the working directory.
const artifactsSubdir = "artifacts"

// tempPrefix starts the name of every working directory created when
// Options.WorkingDir is nil.
const tempPrefix = "aclio-go-browser-"

// timestampLayout is fixed-width UTC with nanoseconds and no colons, so the
// names sort chronologically and are valid on every OS.
const timestampLayout = "20060102T150405.000000000Z"

// ResolveDirs turns o into absolute directories and creates the working
// directory. It creates neither the artifacts directory (made on the first
// write) nor the Chrome data directory (Chrome makes it on launch). A path set
// to "" is an error: leave it nil to get the default.
func ResolveDirs(o Options) (Dirs, error) {
	// Check the cheap things before creating anything on disk.
	if o.ArtifactsDir != nil && *o.ArtifactsDir == "" {
		return Dirs{}, errors.New("ArtifactsDir is set to an empty path; leave it nil for the default")
	}
	var userData *string
	if o.UserDataDir != nil {
		if *o.UserDataDir == "" {
			return Dirs{}, errors.New("UserDataDir is set to an empty path; leave it nil for an anonymous session")
		}
		abs, err := filepath.Abs(*o.UserDataDir)
		if err != nil {
			return Dirs{}, fmt.Errorf("resolving UserDataDir: %w", err)
		}
		userData = &abs
	}

	working, err := resolveWorkingDir(o.WorkingDir)
	if err != nil {
		return Dirs{}, err
	}

	artifacts := filepath.Join(working, artifactsSubdir)
	if o.ArtifactsDir != nil {
		if artifacts, err = filepath.Abs(*o.ArtifactsDir); err != nil {
			return Dirs{}, fmt.Errorf("resolving ArtifactsDir: %w", err)
		}
	}

	return Dirs{Working: working, Artifacts: artifacts, UserData: userData}, nil
}

func resolveWorkingDir(dir *string) (string, error) {
	if dir == nil {
		return newTempWorkingDir(time.Now())
	}
	if *dir == "" {
		return "", errors.New("WorkingDir is set to an empty path; leave it nil for a temp dir")
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return "", fmt.Errorf("resolving WorkingDir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("creating WorkingDir: %w", err)
	}
	return abs, nil
}

// newTempWorkingDir creates aclio-go-browser-{timestamp}-{random} under the
// system temp dir. The random suffix keeps runs started in the same
// nanosecond apart without disturbing the chronological order.
func newTempWorkingDir(now time.Time) (string, error) {
	pattern := tempPrefix + now.UTC().Format(timestampLayout) + "-*"
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("creating temp working dir: %w", err)
	}
	return dir, nil
}
