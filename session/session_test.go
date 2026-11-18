package session

import (
	"strings"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

func ptr[T any](v T) *T { return &v }

func TestResolveGoTo(t *testing.T) {
	cases := []struct{ current, raw, want string }{
		{"https://x.com/a/b", "https://y.com/", "https://y.com/"},
		{"https://x.com/a/b", "/root", "https://x.com/root"},
		{"https://x.com/a/b", "../up", "https://x.com/up"},
		{"https://x.com/a/b", "c?q=1", "https://x.com/a/c?q=1"},
		{"http://x.com/", "#frag", "http://x.com/#frag"},
		{"https://x.com/a", "about:blank", "about:blank"},
		{"about:blank", "https://x.com/", "https://x.com/"},
	}
	for _, c := range cases {
		got, err := resolveGoTo(c.current, c.raw)
		if err != nil || got != c.want {
			t.Errorf("resolveGoTo(%q, %q) = %q, %v; want %q", c.current, c.raw, got, err, c.want)
		}
	}

	bad := []struct{ current, raw string }{
		{"https://x.com/", "javascript:alert(1)"},
		{"https://x.com/", "file:///etc/passwd"},
		{"about:blank", "/relative"},
		{"https://x.com/", "about:config"},
		{"file:///tmp/page.html", "other.html"},
	}
	for _, c := range bad {
		if got, err := resolveGoTo(c.current, c.raw); err == nil {
			t.Errorf("resolveGoTo(%q, %q) = %q, want error", c.current, c.raw, got)
		}
	}
}

func TestValidateOpts(t *testing.T) {
	ok := Opts{VideoDir: "/abs/video", ArtifactsDir: "/abs/art"}
	if err := validateOpts(ok); err != nil {
		t.Fatal(err)
	}
	err := validateOpts(Opts{VideoDir: "rel", UserDataDir: ptr("rel"), Viewport: &Size{Width: 0, Height: 10}})
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"VideoDir", "ArtifactsDir", "UserDataDir", "Viewport"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestOpenRejectsBadOptsBeforeLaunching(t *testing.T) {
	if _, err := Open(Opts{Browser: "webkit", VideoDir: "/v", ArtifactsDir: "/a"}); err == nil {
		t.Error("unsupported browser should fail")
	}
	if _, err := Open(Opts{}); err == nil {
		t.Error("missing dirs should fail")
	}
}

func TestViewportDefault(t *testing.T) {
	if v := viewport(nil); v.Width != 1280 || v.Height != 720 {
		t.Errorf("default viewport = %+v", v)
	}
	if v := viewport(&Size{Width: 800, Height: 600}); v.Width != 800 || v.Height != 600 {
		t.Errorf("viewport = %+v", v)
	}
}

// A closed or nil Browser fails every call cleanly instead of panicking.
func TestClosedBrowser(t *testing.T) {
	for _, b := range []*Browser{nil, {closed: true}} {
		if err := Close(b); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := Navigate(b, "https://x.com"); err == nil {
			t.Error("Navigate should fail")
		}
		if _, err := Tabs(b); err == nil {
			t.Error("Tabs should fail")
		}
		if _, err := Snapshot(b, preset.Perception{Tabs: true}, t.TempDir()); err == nil {
			t.Error("Snapshot should fail")
		}
		got := Perform(b, []action.Action{
			{Type: action.TypeWait, Data: action.Wait{Milliseconds: 1}},
			{Type: action.TypeWait, Data: action.Wait{Milliseconds: 1}},
		})
		if !got[0].Attempted || got[0].Succeeded || got[0].Reason == nil || got[1].Attempted {
			t.Errorf("Perform on a closed browser = %+v", got)
		}
	}
}

// The focused element's roles come from Chromium's accessibility tree, so
// Focused refuses Firefox before touching the page.
func TestFocusedNeedsChromium(t *testing.T) {
	if _, err := Focused(&Browser{engine: engine.Firefox}); err == nil || !strings.Contains(err.Error(), "Chromium") {
		t.Errorf("Focused on Firefox = %v, want a Chromium error", err)
	}
}

// Announcing needs Chromium like Focused, refuses an unsupported language,
// and a browser that isn't announcing has nothing to take.
func TestAnnouncingChecks(t *testing.T) {
	if err := StartAnnouncing(&Browser{engine: engine.Firefox}, ""); err == nil || !strings.Contains(err.Error(), "Chromium") {
		t.Errorf("StartAnnouncing on Firefox = %v, want a Chromium error", err)
	}
	if err := StartAnnouncing(&Browser{engine: engine.Chromium}, "de-DE"); err == nil {
		t.Error("StartAnnouncing should refuse an unsupported language")
	}
	if err := StartAnnouncing(nil, ""); err == nil {
		t.Error("StartAnnouncing on nil should fail")
	}
	if got := TakeAnnouncements(&Browser{}); got != nil {
		t.Errorf("TakeAnnouncements without announcing = %q, want nil", got)
	}
	if got := TakeAnnouncements(nil); got != nil {
		t.Errorf("TakeAnnouncements(nil) = %q", got)
	}
}
