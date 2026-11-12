package browserinstall

import (
	"bytes"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
)

func TestRunOptions(t *testing.T) {
	var buf bytes.Buffer
	o, err := runOptions(Opts{
		Browsers:       []engine.Browser{engine.Firefox},
		Output:         &buf,
		WithSystemDeps: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(o.Browsers, []string{"firefox"}) {
		t.Errorf("Browsers = %v, want [firefox]", o.Browsers)
	}
	if !o.WithDeps {
		t.Error("WithSystemDeps not passed through")
	}
	if o.Stdout != &buf || o.Stderr != &buf {
		t.Error("Output not used for stdout and stderr")
	}
	if o.Logger == nil {
		t.Error("Logger must be set, or playwright-go hijacks the global log package")
	}
	if o.SkipInstallBrowsers {
		t.Error("browsers must be installed")
	}
}

func TestRunOptionsDefaults(t *testing.T) {
	o, err := runOptions(Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(o.Browsers, []string{"chromium"}) {
		t.Errorf("Browsers = %v, want only chromium", o.Browsers)
	}
	if o.Stdout != os.Stderr || o.Stderr != os.Stderr {
		t.Error("nil Output should default to os.Stderr")
	}
	if o.WithDeps {
		t.Error("system deps should be opt-in")
	}
}

func TestBrowserNames(t *testing.T) {
	got, err := browserNames([]engine.Browser{engine.Firefox, "", engine.Chromium, engine.Firefox})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"firefox", "chromium"}) {
		t.Errorf("browserNames = %v, want [firefox chromium]", got)
	}
	if _, err := browserNames([]engine.Browser{"webkit"}); err == nil {
		t.Error("an unsupported browser should be an error")
	}
}

// An unsupported browser must fail before anything is downloaded.
func TestEnsureRejectsUnsupportedBrowser(t *testing.T) {
	if err := Ensure(Opts{Browsers: []engine.Browser{"webkit"}, Output: io.Discard}); err == nil {
		t.Fatal("want error")
	}
}

// TestEnsure really downloads the driver and both browsers (hundreds of MB),
// so it only runs when asked to.
func TestEnsure(t *testing.T) {
	if os.Getenv("ACLIO_BROWSER_INSTALL_TEST") == "" {
		t.Skip("set ACLIO_BROWSER_INSTALL_TEST=1 to download the driver and browsers")
	}
	opts := Opts{Browsers: engine.All(), Output: io.Discard}
	if err := Ensure(opts); err != nil {
		t.Fatal(err)
	}
	// A second call finds everything in place.
	if err := Ensure(opts); err != nil {
		t.Fatal(err)
	}
}
