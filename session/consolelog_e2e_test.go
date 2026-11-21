package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// consoleServer serves a page that logs at several levels, throws, starts a
// worker that logs, opens a popup that logs and then navigates; and a page
// that logs 10,000 messages in a tight loop on load and on every click.
func consoleServer(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/": `<!doctype html><title>Logs</title><script>
console.log('hello log'); console.info('hello info'); console.warn('hello warn');
console.error('hello error'); console.debug('hello debug');
new Worker('/worker.js');
window.open('/popup');
fetch('/api/bad');
setTimeout(() => { throw new TypeError('boom') }, 0);
setTimeout(() => { location.href = '/next' }, 1500);
</script>`,
		"/popup":     `<!doctype html><title>Popup</title><script>console.log('popup says hi')</script>`,
		"/next":      `<!doctype html><title>Next</title><script>console.log('after navigation')</script>`,
		"/worker.js": `console.log('worker says hi')`,
		"/api/bad":   `{"detail":"bad"}`,
		"/flood": `<!doctype html><title>Flood</title><button id=b>b</button><script>
const flood = () => { for (let i = 0; i < 10000; i++) console.log('flood ' + i + ' ' + 'x'.repeat(100)) };
flood(); document.getElementById('b').onclick = flood;
</script>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/bad" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, body)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "text/javascript")
		} else {
			w.Header().Set("Content-Type", "text/html")
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// waitForLog waits until every want is a substring of some line of path.
func waitForLog(t *testing.T, path string, wants ...string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, _ := os.ReadFile(path)
		missing := slices.DeleteFunc(slices.Clone(wants), func(w string) bool { return strings.Contains(string(data), w) })
		if len(missing) == 0 {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("log never got %q:\n%s", missing, data)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func openLogged(t *testing.T, eng engine.Browser, opts Opts) (*Browser, string) {
	t.Helper()
	dir := t.TempDir()
	opts.Browser = eng
	opts.VideoDir = filepath.Join(dir, "video")
	opts.ArtifactsDir = filepath.Join(dir, "artifacts")
	if opts.ConsoleLog == "" {
		opts.ConsoleLog = filepath.Join(dir, "debug", "console.jsonl")
	}
	b, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(b) })
	return b, opts.ConsoleLog
}

func TestE2EConsoleLog(t *testing.T) {
	e2e(t)
	srv := consoleServer(t)
	for _, eng := range engine.All() {
		t.Run(string(eng), func(t *testing.T) {
			b, path := openLogged(t, eng, Opts{})
			if err := Navigate(b, srv.URL); err != nil {
				t.Fatal(err)
			}
			LogTurn(b, 1)
			// Everything is on disk before Close.
			waitForLog(t, path,
				`"text":"hello log"`, `"level":"warning"`, `"level":"error"`, `"level":"debug"`,
				`"text":"worker says hi"`, `"text":"popup says hi"`, `"text":"after navigation"`,
				`"kind":"pageerror"`, `"message":"boom"`, `"kind":"turn"`)

			if err := Close(b); err != nil {
				t.Fatal(err)
			}
			recs := readLog(t, path)
			if k := kinds(recs); k[0] != "open" || k[len(k)-1] != "closed" {
				t.Errorf("kinds = %v", k)
			}
			byText := map[string]map[string]any{}
			for _, r := range recs {
				if s, ok := r["text"].(string); ok {
					byText[s] = r
				}
			}
			if r := byText["hello log"]; r["tab"] != "t1" || r["page"] != srv.URL+"/" || r["worker"] != false {
				t.Errorf("hello log = %v", r)
			}
			if r := byText["popup says hi"]; r["tab"] != "t2" || r["page"] != srv.URL+"/popup" {
				t.Errorf("popup = %v", r)
			}
			if r := byText["after navigation"]; r["tab"] != "t1" || r["page"] != srv.URL+"/next" {
				t.Errorf("after navigation = %v", r)
			}
			if r := byText["worker says hi"]; r["worker"] != true || !strings.HasSuffix(fmt.Sprint(r["workerURL"]), "/worker.js") {
				t.Errorf("worker = %v", r)
			}
			var navs, actives int
			for _, r := range recs {
				switch r["kind"] {
				case "navigation":
					if r["page"] == srv.URL+"/next" && r["tab"] == "t1" {
						navs++
					}
				case "active":
					actives++
				case "pageerror":
					if r["name"] != "TypeError" || r["tab"] != "t1" {
						t.Errorf("pageerror = %v", r)
					}
				}
			}
			if navs != 1 || actives != 1 {
				t.Errorf("navigations to /next = %d, active records = %d; want 1, 1", navs, actives)
			}
		})
	}
}

// Chromium crashes a tab that navigates to chrome://crash.
func TestE2EConsoleLogCrash(t *testing.T) {
	e2e(t)
	b, path := openLogged(t, engine.Chromium, Opts{})
	_ = Navigate(b, "chrome://crash") // fails: the page crashed
	waitForLog(t, path, `"kind":"crash","tab":"t1"`)
}

func TestE2EConsoleLogStress(t *testing.T) {
	e2e(t)
	srv := consoleServer(t)
	b, path := openLogged(t, engine.Chromium, Opts{ConsoleLogMaxBytes: 1 << 20, ConsoleLogFieldCap: 50})
	if err := Navigate(b, srv.URL+"/flood"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for range 5 {
		done := Perform(b, []action.Action{
			{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("#b")}}},
			{Type: action.TypePress, Data: action.Press{Key: "Tab"}},
		})
		for _, d := range done {
			if !d.Succeeded {
				t.Fatalf("%v failed: %s", d.Action.Type, *d.Reason)
			}
		}
		if _, err := Snapshot(b, preset.Perception{Tabs: true}, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("actions took %v under the flood", d)
	}
	if err := Close(b); err != nil {
		t.Fatal(err)
	}
	recs := readLog(t, path)
	fi, _ := os.Stat(path)
	if fi.Size() > 1<<20+1000 {
		t.Errorf("size %d is past the cap", fi.Size())
	}
	var dropped int
	for _, r := range recs {
		if r["kind"] == "dropped" {
			dropped++
		}
		if s, ok := r["text"].(string); ok && len(s) > 50 {
			t.Fatalf("text not capped: %q", s)
		}
	}
	last := recs[len(recs)-1]
	if dropped != 1 || last["kind"] != "closed" || last["dropped"].(float64) == 0 {
		t.Errorf("dropped records = %d, closed = %v", dropped, last)
	}
}

// TestE2EConsoleLogKill kills a process mid-session and checks that every
// record it had written, to the console log and the network log, is still
// there.
func TestE2EConsoleLogKill(t *testing.T) {
	e2e(t)
	srv := consoleServer(t)
	path := filepath.Join(t.TempDir(), "console.jsonl")
	cmd := exec.Command(os.Args[0], "-test.run=^TestConsoleLogKillHelper$", "-test.v")
	netPath := filepath.Join(filepath.Dir(path), "network.jsonl")
	cmd.Env = append(os.Environ(), "ACLIO_CONSOLE_LOG_HELPER="+path, "ACLIO_NETWORK_LOG_HELPER="+netPath, "ACLIO_CONSOLE_LOG_URL="+srv.URL)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	before := waitForLog(t, path, `"text":"hello log"`, `"text":"after navigation"`, `"kind":"pageerror"`)
	netBefore := waitForLog(t, netPath, `"kind":"response"`, `"status":400`)
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), before) {
		t.Errorf("records lost by the kill:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	recs := readLog(t, path) // every line is JSON
	if k := kinds(recs); slices.Contains(k, "closed") {
		t.Errorf("a killed run wrote closed: %v", k)
	}
	netAfter, _ := os.ReadFile(netPath)
	if !strings.HasPrefix(string(netAfter), netBefore) {
		t.Errorf("network records lost by the kill:\nbefore:\n%s\nafter:\n%s", netBefore, netAfter)
	}
	readLog(t, netPath)
}

// TestConsoleLogKillHelper is TestE2EConsoleLogKill's child process: it
// opens a logged session, loads the page and blocks until killed.
func TestConsoleLogKillHelper(t *testing.T) {
	path := os.Getenv("ACLIO_CONSOLE_LOG_HELPER")
	if path == "" {
		t.Skip("run by TestE2EConsoleLogKill")
	}
	dir := t.TempDir()
	b, err := Open(Opts{
		VideoDir:     filepath.Join(dir, "video"),
		ArtifactsDir: filepath.Join(dir, "artifacts"),
		ConsoleLog:   path,
		NetworkLog:   os.Getenv("ACLIO_NETWORK_LOG_HELPER"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Navigate(b, os.Getenv("ACLIO_CONSOLE_LOG_URL")); err != nil {
		t.Fatal(err)
	}
	select {}
}
