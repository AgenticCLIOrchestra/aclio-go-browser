package session

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// closedPort returns a local port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// networkServer serves /net, a page whose fetches fail every way the network
// log records, plus a worker and a popup that fail fetches of their own.
// /api/slow?nav is cancelled by a navigation, which Playwright doesn't
// report at all; /api/slow is aborted explicitly, which it does.
func networkServer(t *testing.T) *httptest.Server {
	t.Helper()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "no CORS headers")
	}))
	t.Cleanup(other.Close)
	port := closedPort(t)
	big := `{"title":"Bad Request","detail":"` + strings.Repeat("d", 20000) + `","traceId":"abc"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/net":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "cookie-secret"})
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!doctype html><title>Net</title><script>
fetch('/api/bad', {method: 'POST', body: 'login-secret'});
fetch('/api/big', {method: 'POST', body: 'opt-in-body'});
fetch('/api/png'); fetch('/api/ok');
fetch('http://127.0.0.1:%d/refused').catch(() => {});
fetch('%s/nocors').catch(() => {});
new Worker('/worker.js');
window.open('/popup');
const ac = new AbortController();
fetch('/api/slow', {signal: ac.signal}).catch(() => {});
setTimeout(() => ac.abort(), 300);
fetch('/api/slow?nav').catch(() => {});
setTimeout(() => { location.href = '/next' }, 1000);
</script>`, port, other.URL)
		case "/popup":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><script>fetch('/api/popup404')</script>`)
		case "/worker.js":
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprint(w, `fetch('/api/worker404')`)
		case "/next":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><script>fetch('/api/after')</script>`)
		case "/api/bad":
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("X-Trace-Id", "trace-1")
			w.Header().Set("Set-Cookie", "other=cookie-secret")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"title":"Bad Request","detail":"no","traceId":"trace-1"}`)
		case "/api/big":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(422)
			fmt.Fprint(w, big)
		case "/api/png":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(500)
			fmt.Fprint(w, "\x89PNG")
		case "/api/ok":
			fmt.Fprint(w, "fine")
		case "/api/slow":
			select {
			case <-time.After(10 * time.Second):
			case <-r.Context().Done():
			}
		case "/api/slowbad": // the headers at once, the body slowly
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(500)
			w.(http.Flusher).Flush()
			select {
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
			}
			fmt.Fprint(w, "slow body")
		case "/flood":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><button id=b>b</button><script>
for (let i = 0; i < 10; i++) fetch('/api/slowbad?i=' + i).catch(() => {});
for (let i = 0; i < 490; i++) fetch('/api/nope?i=' + i).catch(() => {});
</script>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestE2ENetworkLog(t *testing.T) {
	e2e(t)
	srv := networkServer(t)
	for _, eng := range engine.All() {
		t.Run(string(eng), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "network.jsonl")
			b, _ := openLogged(t, eng, Opts{NetworkLog: path, ConsoleLog: filepath.Join(dir, "console.jsonl"), Network: NetworkLogOpts{
				Headers:         []string{"set-cookie", "authorization"},
				RequestBodyURLs: []string{`/api/big$`},
			}})
			if err := Navigate(b, srv.URL+"/net?code=abc&state=xyz"); err != nil {
				t.Fatal(err)
			}
			// Everything is on disk before Close.
			waitForLog(t, path, `/api/bad"`, `"kind":"responsebody"`, `/api/png"`, `/refused"`, `/nocors"`,
				`/api/popup404"`, `/api/worker404"`, `/api/slow"`, `/api/after"`)
			if err := Close(b); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			for _, secret := range []string{"cookie-secret", "login-secret", "code=abc", "state=xyz", `/api/ok"`} {
				if strings.Contains(string(data), secret) {
					t.Errorf("the log has %q", secret)
				}
			}
			recs := readLog(t, path)
			byURL := map[string]map[string]any{}
			for _, r := range recs {
				if u, ok := r["url"].(string); ok && r["kind"] != "finished" {
					byURL[strings.TrimPrefix(u, srv.URL)] = r
				}
			}
			if r := byURL["/api/bad"]; r["status"] != 400.0 || r["tab"] != "t1" || r["page"] != srv.URL+"/net?code=REDACTED&state=REDACTED" {
				t.Errorf("bad = %v", r)
			} else if h := r["headers"].(map[string]any); h["x-trace-id"] != "trace-1" || h["set-cookie"] != nil {
				t.Errorf("bad headers = %v", h)
			}
			if r := byURL["/api/big"]; r["requestBody"] != "opt-in-body" {
				t.Errorf("big = %v", r)
			}
			if r := byURL["/api/png"]; r["bodyOmitted"] != "content-type" {
				t.Errorf("png = %v", r)
			}
			if r := byURL["/api/popup404"]; r["tab"] != "t2" || r["status"] != 404.0 {
				t.Errorf("popup = %v", r)
			}
			if r := byURL["/api/worker404"]; r["status"] != 404.0 {
				t.Errorf("worker = %v", r)
			}
			if r := byURL["/api/after"]; r["page"] != srv.URL+"/next" {
				t.Errorf("after = %v", r)
			}
			if r := byURL["/api/slow"]; r["kind"] != "requestfailed" || r["aborted"] != true {
				t.Errorf("slow = %v", r)
			}
			for _, u := range []string{"/nocors", "/refused"} {
				for k, r := range byURL {
					if strings.HasSuffix(k, u) && (r["kind"] != "requestfailed" || r["aborted"] != false) {
						t.Errorf("%s = %v", u, r)
					}
				}
			}
			bodies := byKind(recs, "responsebody")
			ids := map[float64]map[string]any{}
			for _, r := range bodies {
				ids[r["id"].(float64)] = r
			}
			if r := ids[byURL["/api/bad"]["id"].(float64)]; !strings.Contains(fmt.Sprint(r["body"]), `"traceId":"trace-1"`) {
				t.Errorf("bad body = %v", r)
			}
			if r := ids[byURL["/api/big"]["id"].(float64)]; r["truncated"] != true || len(r["body"].(string)) != DefaultNetworkLogBodyCap {
				t.Errorf("big body truncated = %v, len %d", r["truncated"], len(fmt.Sprint(r["body"])))
			}
			if last := recs[len(recs)-1]; last["kind"] != "closed" {
				t.Errorf("last = %v", last)
			}
		})
	}
}

func TestE2ENetworkLogAll(t *testing.T) {
	e2e(t)
	srv := networkServer(t)
	path := filepath.Join(t.TempDir(), "network.jsonl")
	b, _ := openLogged(t, engine.Chromium, Opts{NetworkLog: path, Network: NetworkLogOpts{Mode: NetworkLogAll}})
	if err := Navigate(b, srv.URL+"/next"); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, path, `/api/after"`)
	if err := Close(b); err != nil {
		t.Fatal(err)
	}
	recs := readLog(t, path)
	var resp, fin map[string]any
	for _, r := range recs {
		if r["url"] == srv.URL+"/next" {
			switch r["kind"] {
			case "response":
				resp = r
			case "finished":
				fin = r
			}
		}
	}
	if resp == nil || fin == nil || resp["status"] != 200.0 || resp["id"] != fin["id"] || fin["durationMs"] == nil {
		t.Errorf("response = %v, finished = %v", resp, fin)
	}
}

// 500 failing requests, some with slow bodies, don't stall actions; the
// queue skips bodies rather than block, and Close's drain is bounded. (Only
// a few slow ones: the browser opens 6 connections per host.)
func TestE2ENetworkLogStress(t *testing.T) {
	e2e(t)
	srv := networkServer(t)
	path := filepath.Join(t.TempDir(), "network.jsonl")
	b, _ := openLogged(t, engine.Chromium, Opts{NetworkLog: path, Network: NetworkLogOpts{BodyTimeout: time.Second}})
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
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("actions took %v", d)
	}
	waitForLog(t, path, `"bodySkipped":"queue full"`)
	start = time.Now()
	if err := Close(b); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("Close took %v", d)
	}
	recs := readLog(t, path)
	last := recs[len(recs)-1]
	if last["kind"] != "closed" || last["pending"] == nil {
		t.Errorf("closed = %v", last)
	}
	t.Logf("responses %d, bodies %d, closed %v", len(byKind(recs, "response")), len(byKind(recs, "responsebody")), last)
}
