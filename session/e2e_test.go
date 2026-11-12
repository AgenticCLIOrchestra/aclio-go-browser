package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// These tests drive real browsers, so they need the Playwright driver and
// both browsers installed (browserinstall.Ensure). They run only when
// ACLIO_BROWSER_E2E=1, and CI does not set it.

const testPage = `<!doctype html>
<title>Form</title>
<label for="name">Name</label><input id="name">
<button id="go" onclick="document.title = 'Hi ' + document.getElementById('name').value">Go</button>
<a href="/other">other</a>`

func e2e(t *testing.T) {
	t.Helper()
	if os.Getenv("ACLIO_BROWSER_E2E") == "" {
		t.Skip("set ACLIO_BROWSER_E2E=1 to drive real browsers (needs browserinstall.Ensure)")
	}
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/other" {
			fmt.Fprint(w, "<title>Other</title><p>other page</p>")
			return
		}
		fmt.Fprint(w, testPage)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func open(t *testing.T, eng engine.Browser) (*Browser, string) {
	t.Helper()
	dir := t.TempDir()
	b, err := Open(Opts{
		Browser:      eng,
		VideoDir:     filepath.Join(dir, "video"),
		ArtifactsDir: filepath.Join(dir, "artifacts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(b) })
	return b, dir
}

func TestE2E(t *testing.T) {
	e2e(t)
	srv := testServer(t)

	for _, eng := range engine.All() {
		t.Run(string(eng), func(t *testing.T) {
			b, dir := open(t, eng)
			if err := Navigate(b, srv.URL); err != nil {
				t.Fatal(err)
			}

			done := Perform(b, []action.Action{
				{Type: action.TypeFill, Data: action.Fill{Target: action.Target{Selector: ptr("#name")}, Value: "Ada"}},
				{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("#go")}}},
				{Type: action.TypeJavascript, Data: action.Javascript{Code: "document.title"}},
				{Type: action.TypeCreateArtifact, Data: action.CreateArtifact{Name: "notes.md", Content: "hi"}},
				{Type: action.TypeScreenshot, Data: action.Screenshot{Name: "page", FullPage: true}},
				{Type: action.TypeListArtifacts, Data: action.ListArtifacts{}},
				{Type: action.TypeTabOpen, Data: action.TabOpen{}},
				{Type: action.TypeTabSwitch, Data: action.TabSwitch{Index: 1}},
				{Type: action.TypeGoTo, Data: action.GoTo{URL: "/other"}},
			})
			for _, p := range done {
				if !p.Succeeded {
					t.Fatalf("%s failed: %v", p.Type, *p.Reason)
				}
			}
			if got := *done[2].Result; got != `"Hi Ada"` {
				t.Errorf("javascript result = %s", got)
			}
			if got := *done[5].Result; got != `["notes.md","page.png"]` {
				t.Errorf("listArtifacts = %s", got)
			}

			st, err := Snapshot(b, preset.Perception{Screenshot: true, DOM: true, Focused: true, Tabs: true}, filepath.Join(dir, "turns"))
			if err != nil {
				t.Fatal(err)
			}
			if len(st.Tabs) != 2 || !st.Tabs[0].Active || !strings.HasSuffix(st.Tabs[0].URL, "/other") {
				t.Errorf("tabs = %+v", st.Tabs)
			}
			for _, p := range []string{st.ScreenshotPath, st.DOMPath} {
				if info, err := os.Stat(p); err != nil || info.Size() == 0 {
					t.Errorf("snapshot file %q: %v", p, err)
				}
			}

			// Refusals come back as reasons, not errors.
			bad := Perform(b, []action.Action{
				{Type: action.TypeCreateArtifact, Data: action.CreateArtifact{Name: "notes.md", Content: "again"}},
			})
			if bad[0].Succeeded || bad[0].Reason == nil {
				t.Error("overwriting an artifact should fail")
			}

			if err := Close(b); err != nil {
				t.Fatal(err)
			}
			videos, _ := os.ReadDir(filepath.Join(dir, "video"))
			if len(videos) == 0 {
				t.Error("no video was recorded")
			}
		})
	}
}

// Two browsers in parallel share nothing.
func TestE2EConcurrentBrowsers(t *testing.T) {
	e2e(t)
	srv := testServer(t)

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			b, _ := open(t, engine.Chromium)
			if err := Navigate(b, srv.URL); err != nil {
				t.Error(err)
				return
			}
			name := fmt.Sprintf("run-%d", i)
			done := Perform(b, []action.Action{
				{Type: action.TypeFill, Data: action.Fill{Target: action.Target{Selector: ptr("#name")}, Value: name}},
				{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("#go")}}},
			})
			if !done[1].Succeeded {
				t.Errorf("%s: %v", name, *done[len(done)-1].Reason)
				return
			}
			tabs, err := Tabs(b)
			if err != nil || len(tabs) != 1 || tabs[0].Title != "Hi "+name {
				t.Errorf("%s: tabs = %+v, %v", name, tabs, err)
			}
		})
	}
	wg.Wait()
}
