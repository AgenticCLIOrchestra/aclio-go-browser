package session

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// fakeRequest and fakeResponse are a playwright.Request / Response with only
// what the network log uses.
type fakeRequest struct {
	playwright.Request
	method, url, failure string
	post                 []byte
	timing               *playwright.RequestTiming
}

func (r *fakeRequest) Method() string                    { return r.method }
func (r *fakeRequest) URL() string                       { return r.url }
func (r *fakeRequest) ResourceType() string              { return "fetch" }
func (r *fakeRequest) Frame() playwright.Frame           { return nil }
func (r *fakeRequest) Timing() *playwright.RequestTiming { return r.timing }
func (r *fakeRequest) PostDataBuffer() ([]byte, error)   { return r.post, nil }
func (r *fakeRequest) Failure() error {
	if r.failure == "" {
		return nil
	}
	return errors.New(r.failure)
}

type fakeResponse struct {
	playwright.Response
	req     *fakeRequest
	status  int
	headers map[string]string
	body    func() ([]byte, error)
}

func (r *fakeResponse) Status() int                 { return r.status }
func (r *fakeResponse) StatusText() string          { return "Status" }
func (r *fakeResponse) FromServiceWorker() bool     { return false }
func (r *fakeResponse) Headers() map[string]string  { return r.headers }
func (r *fakeResponse) Request() playwright.Request { return r.req }
func (r *fakeResponse) Body() ([]byte, error)       { return r.body() }

func netLogs(t *testing.T, n NetworkLogOpts) (*debugLogs, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "network.jsonl")
	d := startLogs(Opts{NetworkLog: path, Network: n}, engine.Chromium)
	if d == nil || d.network == nil || d.console != nil {
		t.Fatal("expected a network log only")
	}
	return d, path
}

func byKind(recs []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if r["kind"] == kind {
			out = append(out, r)
		}
	}
	return out
}

func TestNetworkLogFailures(t *testing.T) {
	d, path := netLogs(t, NetworkLogOpts{BodyCap: 10, RequestBodyURLs: []string{`/opt-in$`}})
	timing := &playwright.RequestTiming{ResponseStart: 142, ResponseEnd: 150}
	ok := &fakeResponse{req: &fakeRequest{method: "GET", url: "http://x/ok"}, status: 200}
	bad := &fakeResponse{
		req:     &fakeRequest{method: "POST", url: "http://x/api?token=abc&x=1", post: []byte("secret"), timing: timing},
		status:  400,
		headers: map[string]string{"content-type": "application/problem+json", "x-trace-id": "tr", "set-cookie": "s", "server": "nginx"},
		body:    func() ([]byte, error) { return []byte(`{"detail":"long detail"}`), nil },
	}
	png := &fakeResponse{req: &fakeRequest{method: "GET", url: "http://x/img"}, status: 500, headers: map[string]string{"content-type": "image/png"}}
	optIn := &fakeResponse{req: &fakeRequest{method: "POST", url: "http://x/opt-in", post: []byte("form=1")}, status: 404,
		headers: map[string]string{"content-type": "text/plain"}, body: func() ([]byte, error) { return nil, errors.New("gone") }}
	logResponse(d, ok)
	logResponse(d, bad)
	logResponse(d, png)
	logResponse(d, optIn)
	logRequestEnd(d, "requestfailed", &fakeRequest{method: "GET", url: "http://x/nav", failure: "net::ERR_ABORTED"})
	logRequestEnd(d, "requestfailed", &fakeRequest{method: "GET", url: "http://127.0.0.1:1/", failure: "net::ERR_CONNECTION_REFUSED"})

	// The response records are on disk at once; the bodies follow.
	waitForLog(t, path, `"kind":"responsebody","id":1`, `"kind":"responsebody","id":3`)
	drainNetwork(d)
	stopLogs(d)
	recs := readLog(t, path)

	resps := byKind(recs, "response")
	if len(resps) != 3 {
		t.Fatalf("responses = %v (a 200 isn't a failure)", resps)
	}
	r := resps[0]
	if r["id"] != 1.0 || r["status"] != 400.0 || r["method"] != "POST" || r["url"] != "http://x/api?token=REDACTED&x=1" || r["ttfbMs"] != 142.0 {
		t.Errorf("400 = %v", r)
	}
	if h := r["headers"].(map[string]any); len(h) != 2 || h["x-trace-id"] != "tr" || h["content-type"] == nil {
		t.Errorf("headers = %v", h)
	}
	if _, ok := r["requestBody"]; ok {
		t.Errorf("request body recorded without opting in: %v", r)
	}
	if resps[1]["bodyOmitted"] != "content-type" {
		t.Errorf("png = %v", resps[1])
	}
	if resps[2]["requestBody"] != "form=1" {
		t.Errorf("opted-in body = %v", resps[2])
	}
	bodies := byKind(recs, "responsebody")
	if len(bodies) != 2 || bodies[0]["body"] != `{"detail":` || bodies[0]["truncated"] != true || bodies[0]["contentType"] != "application/problem+json" {
		t.Errorf("bodies = %v", bodies)
	}
	if !strings.HasPrefix(bodies[1]["error"].(string), "body unavailable: gone") {
		t.Errorf("unavailable body = %v", bodies[1])
	}
	fails := byKind(recs, "requestfailed")
	if len(fails) != 2 || fails[0]["aborted"] != true || fails[1]["aborted"] != false || fails[1]["failure"] != "net::ERR_CONNECTION_REFUSED" {
		t.Errorf("failures = %v", fails)
	}
	last := recs[len(recs)-1]
	if last["kind"] != "closed" || last["pending"] != 0.0 {
		t.Errorf("closed = %v", last)
	}
}

func TestNetworkLogAll(t *testing.T) {
	d, path := netLogs(t, NetworkLogOpts{Mode: NetworkLogAll})
	req := &fakeRequest{method: "GET", url: "http://x/ok", timing: &playwright.RequestTiming{ResponseStart: 5, ResponseEnd: 9}}
	logResponse(d, &fakeResponse{req: req, status: 200})
	logRequestEnd(d, "finished", req)
	drainNetwork(d)
	stopLogs(d)
	recs := readLog(t, path)
	resp, fin := byKind(recs, "response"), byKind(recs, "finished")
	if len(resp) != 1 || len(fin) != 1 || resp[0]["id"] != fin[0]["id"] || fin[0]["durationMs"] != 9.0 {
		t.Errorf("response = %v, finished = %v", resp, fin)
	}
	if len(d.network.pending) != 0 {
		t.Errorf("pending = %v", d.network.pending)
	}
}

// In failures mode too, a recorded response and its request's failure share
// an id, and a finished request is forgotten.
func TestNetworkLogLinksFailureToResponse(t *testing.T) {
	d, path := netLogs(t, NetworkLogOpts{})
	broken := &fakeRequest{method: "GET", url: "http://x/broken", failure: "net::ERR_CONNECTION_RESET"}
	done := &fakeRequest{method: "GET", url: "http://x/done"}
	logResponse(d, &fakeResponse{req: broken, status: 500, headers: map[string]string{"content-type": "image/png"}})
	logResponse(d, &fakeResponse{req: done, status: 404, headers: map[string]string{"content-type": "image/png"}})
	logRequestEnd(d, "requestfailed", broken)
	forgetRequest(d.network, done)
	if len(d.network.pending) != 0 {
		t.Errorf("pending = %v", d.network.pending)
	}
	drainNetwork(d)
	stopLogs(d)
	recs := readLog(t, path)
	resp, fail := byKind(recs, "response"), byKind(recs, "requestfailed")
	if len(fail) != 1 || fail[0]["id"] != resp[0]["id"] {
		t.Errorf("response = %v, requestfailed = %v", resp, fail)
	}
}

// A body fetch that times out keeps its queue slot until Body returns.
func TestNetworkLogTimedOutFetchKeepsSlot(t *testing.T) {
	d, path := netLogs(t, NetworkLogOpts{QueueSize: 1, BodyTimeout: 50 * time.Millisecond})
	release := make(chan struct{})
	hang := &fakeResponse{req: &fakeRequest{method: "GET", url: "http://x/hang"}, status: 500,
		headers: map[string]string{"content-type": "text/plain"}, body: func() ([]byte, error) { <-release; return nil, errors.New("closed") }}
	logResponse(d, hang)
	waitForLog(t, path, `timed out after 50ms`)
	logResponse(d, hang) // the timed-out fetch still holds the only slot
	close(release)
	for len(d.network.slots) != 0 { // freed once Body returns
		time.Sleep(10 * time.Millisecond)
	}
	logResponse(d, hang) // queued again
	drainNetwork(d)
	stopLogs(d)
	resps := byKind(readLog(t, path), "response")
	if len(resps) != 3 || resps[0]["bodySkipped"] != nil || resps[1]["bodySkipped"] != "queue full" || resps[2]["bodySkipped"] != nil {
		t.Errorf("responses = %v", resps)
	}
}

// A full body queue skips the body instead of blocking, and Close's drain
// window reports what is still being fetched.
func TestNetworkLogQueue(t *testing.T) {
	d, path := netLogs(t, NetworkLogOpts{QueueSize: 2, BodyTimeout: 3 * time.Second})
	release := make(chan struct{})
	slow := func() ([]byte, error) { <-release; return []byte("late"), nil }
	resp := func(u string) *fakeResponse {
		return &fakeResponse{req: &fakeRequest{method: "GET", url: u}, status: 503,
			headers: map[string]string{"content-type": "text/html"}, body: slow}
	}
	start := time.Now()
	logResponse(d, resp("http://x/1"))
	waitForLen(t, d.network)           // the worker took it: fetching
	logResponse(d, resp("http://x/2")) // queued
	logResponse(d, resp("http://x/3")) // full
	if time.Since(start) > time.Second {
		t.Error("a full queue blocked the handler")
	}
	drainNetwork(d) // waits networkDrainWindow, then gives up
	stopLogs(d)
	close(release)
	recs := readLog(t, path)
	resps := byKind(recs, "response")
	if len(resps) != 3 || resps[2]["bodySkipped"] != "queue full" || resps[1]["bodySkipped"] != nil {
		t.Errorf("responses = %v", resps)
	}
	if last := recs[len(recs)-1]; last["pending"] != 2.0 {
		t.Errorf("closed = %v, want 2 pending", last)
	}
	// After the drain, new error bodies aren't queued.
	logResponse(d, resp("http://x/4"))
}

// waitForLen waits until n's worker has taken the one queued job.
func waitForLen(t *testing.T, n *netLog) {
	t.Helper()
	for range 100 {
		if len(n.jobs) == 0 && len(n.slots) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the body worker never took the job")
}

func TestRedactURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://x/cb?code=abc&state=xyz":                  "http://x/cb?code=REDACTED&state=REDACTED",
		"http://x/?a=1&b=2":                               "http://x/?a=1&b=2",
		"http://x/#access_token=t&token_type=Bearer":      "http://x/#access_token=REDACTED&token_type=REDACTED",
		"http://x/#/login?code=1&next=/home":              "http://x/#/login?code=REDACTED&next=/home",
		"http://x/?API_KEY=k&my%5Fsecret=s&sig=1&sig=2&q": "http://x/?API_KEY=REDACTED&my%5Fsecret=REDACTED&sig=REDACTED&sig=REDACTED&q",
		"http://x/?keyboard=1&session=s&csrfToken=c":      "http://x/?keyboard=1&session=REDACTED&csrfToken=REDACTED",
		"http://x/path#section":                           "http://x/path#section",
		"about:blank":                                     "about:blank",
	} {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAllowedHeaders(t *testing.T) {
	h := map[string]string{
		"Content-Type": "a", "content-length": "1", "location": "/", "x-b3-traceid": "t",
		"x-request-id": "r", "x-correlation-id": "c", "server": "s", "x-custom": "c",
		"authorization": "no", "cookie": "no", "set-cookie": "no", "x-api-key": "no",
		"x-auth-token": "no", "proxy-authorization": "no",
	}
	extra := map[string]bool{"x-custom": true, "authorization": true, "set-cookie": true}
	got := allowedHeaders(h, extra)
	for _, k := range []string{"content-type", "content-length", "location", "x-b3-traceid", "x-request-id", "x-correlation-id", "x-custom"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%s missing", k)
		}
	}
	if len(got) != 7 {
		t.Errorf("got = %v", got)
	}
	if allowedHeaders(map[string]string{"server": "s"}, nil) != nil {
		t.Error("no allowed headers should be nil")
	}
}

func TestIsTextContentType(t *testing.T) {
	for ct, want := range map[string]bool{
		"application/json": true, "application/problem+json; charset=utf-8": true, "text/html": true,
		"application/xml": true, "application/atom+xml": true, "TEXT/PLAIN": true,
		"image/png": false, "application/octet-stream": false, "": false, "application/javascript": false,
	} {
		if got := isTextContentType(ct); got != want {
			t.Errorf("isTextContentType(%q) = %v", ct, got)
		}
	}
}

func TestValidateNetworkLog(t *testing.T) {
	errs := validateNetworkLog(Opts{NetworkLog: "rel", Network: NetworkLogOpts{
		Mode: "some", BodyCap: -1, BodyTimeout: -1, QueueSize: -1, FieldCap: -1, MaxBytes: -1, RequestBodyURLs: []string{"("},
	}})
	if len(errs) != 8 {
		t.Errorf("errs = %v", errs)
	}
	if errs := validateNetworkLog(Opts{NetworkLog: "/abs", Network: NetworkLogOpts{Mode: NetworkLogAll, RequestBodyURLs: []string{`/login$`}}}); len(errs) != 0 {
		t.Errorf("errs = %v", errs)
	}
}

// Both logs share the tab ids and get the tab and turn records.
func TestDebugLogsShareTabs(t *testing.T) {
	dir := t.TempDir()
	cpath, npath := filepath.Join(dir, "c.jsonl"), filepath.Join(dir, "n.jsonl")
	d := startLogs(Opts{ConsoleLog: cpath, NetworkLog: npath}, engine.Chromium)
	p := &fakePage{url: "http://x/?code=1"}
	watchPage(d, p)
	LogTurn(&Browser{logs: d}, 1)
	logTab(d, "active", p)
	drainNetwork(d)
	stopLogs(d)
	for _, path := range []string{cpath, npath} {
		recs := readLog(t, path)
		if got := strings.Join(kinds(recs), ","); got != "open,pageopen,turn,active,closed" {
			t.Errorf("%s: kinds = %s", path, got)
		}
		if recs[1]["tab"] != "t1" || recs[1]["page"] != "http://x/?code=REDACTED" {
			t.Errorf("%s: pageopen = %v", path, recs[1])
		}
	}
}
