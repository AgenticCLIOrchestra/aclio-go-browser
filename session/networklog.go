package session

import (
	"fmt"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// NetworkLogMode is what the network log records.
type NetworkLogMode string

const (
	// NetworkLogFailures records responses with status 400 or above (with
	// the start of a text body) and requests that got no response. It is
	// the default.
	NetworkLogFailures NetworkLogMode = "failures"
	// NetworkLogAll also records a one-line summary of every response, and a
	// finished record with the duration of every request.
	NetworkLogAll NetworkLogMode = "all"
)

// Network log defaults, used when the NetworkLogOpts fields are 0.
const (
	DefaultNetworkLogBodyCap     = 8 << 10 // bytes of an error response body, or an opted-in request body
	DefaultNetworkLogBodyTimeout = 5 * time.Second
	DefaultNetworkLogQueueSize   = 64 // error bodies waiting or being fetched
)

// networkDrainWindow is how long Close waits for the error bodies already
// queued, before the context (and with it every body) goes away.
const networkDrainWindow = 2 * time.Second

// NetworkLogOpts configures the network log (Opts.NetworkLog). The zero
// value records failures, with the defaults.
type NetworkLogOpts struct {
	// Mode is what is recorded. "" means NetworkLogFailures.
	Mode NetworkLogMode
	// BodyCap caps an error response body, and an opted-in request body, in
	// bytes. 0 means DefaultNetworkLogBodyCap.
	BodyCap int
	// BodyTimeout is how long fetching one error body may take. 0 means
	// DefaultNetworkLogBodyTimeout.
	BodyTimeout time.Duration
	// QueueSize is how many error bodies may be waiting or being fetched;
	// past it, a response is recorded with "bodySkipped" instead of blocking. 0 means
	// DefaultNetworkLogQueueSize.
	QueueSize int
	// Headers are response header names recorded on top of the default
	// allowlist (content-type, content-length, location, and any name
	// containing "trace", "request-id" or "correlation"). Authorization,
	// Proxy-Authorization, Cookie, Set-Cookie, X-Api-Key and X-Auth-Token are
	// never recorded, even when named here.
	Headers []string
	// RequestBodyURLs are regular expressions; the body of a request whose URL
	// matches one is recorded (capped at BodyCap). No request body is
	// recorded otherwise.
	RequestBodyURLs []string
	// FieldCap caps each URL, in bytes, and MaxBytes caps the file, as for
	// the console log. 0 means DefaultConsoleLogFieldCap /
	// DefaultConsoleLogMaxBytes.
	FieldCap int
	MaxBytes int64
}

// netLog is the network log's state, next to its file.
type netLog struct {
	file        *logFile
	all         bool
	bodyCap     int
	bodyTimeout time.Duration
	headers     map[string]bool // the caller's extra allowlisted names
	bodyURLs    []*regexp.Regexp

	nextID atomic.Int64

	mu        sync.Mutex
	accepting bool                         // jobs may be queued (false once Close drains)
	pending   map[playwright.Request]int64 // all mode: response id, for its finished record
	slots     chan struct{}                // one per body job queued or being fetched
	jobs      chan bodyJob
	done      chan struct{} // closed when the body worker exits
}

// bodyJob is an error body to fetch, for the response record id.
type bodyJob struct {
	id          int64
	resp        playwright.Response
	contentType string
}

// newNetLog sets up the network log on f, opts already validated, and
// starts its body worker.
func newNetLog(f *logFile, opts NetworkLogOpts) *netLog {
	n := &netLog{
		file:        f,
		all:         opts.Mode == NetworkLogAll,
		bodyCap:     opts.BodyCap,
		bodyTimeout: opts.BodyTimeout,
		headers:     map[string]bool{},
		accepting:   true,
		pending:     map[playwright.Request]int64{},
		done:        make(chan struct{}),
	}
	if n.bodyCap == 0 {
		n.bodyCap = DefaultNetworkLogBodyCap
	}
	if n.bodyTimeout == 0 {
		n.bodyTimeout = DefaultNetworkLogBodyTimeout
	}
	size := opts.QueueSize
	if size == 0 {
		size = DefaultNetworkLogQueueSize
	}
	n.slots, n.jobs = make(chan struct{}, size), make(chan bodyJob, size)
	for _, h := range opts.Headers {
		n.headers[strings.ToLower(h)] = true
	}
	for _, p := range opts.RequestBodyURLs {
		if re, err := regexp.Compile(p); err == nil {
			n.bodyURLs = append(n.bodyURLs, re)
		}
	}
	go runBodyWorker(n)
	return n
}

// listenNetwork subscribes d's network log to ctx's responses, failed
// requests and (in all mode) finished requests, from every tab, popup,
// worker and service worker. Recording is passive: nothing is intercepted.
func listenNetwork(d *debugLogs, ctx playwright.BrowserContext) {
	ctx.OnResponse(func(r playwright.Response) { logResponse(d, r) })
	ctx.OnRequestFailed(func(r playwright.Request) { logRequestEnd(d, "requestfailed", r) })
	if d.network.all {
		ctx.OnRequestFinished(func(r playwright.Request) { logRequestEnd(d, "finished", r) })
	}
}

// requestRecord is the part of a record every request kind shares, from
// local accessors only.
func requestRecord(d *debugLogs, kind string, req playwright.Request) logRecord {
	r := logRecord{Kind: kind, Method: req.Method(), URL: strPtr(req.URL()), ResourceType: req.ResourceType()}
	if f := req.Frame(); f != nil {
		pageOf(d, &r, f.Page())
	}
	if body := requestBody(d.network, req); body != nil {
		r.RequestBody = body
	}
	return r
}

// logResponse writes a response record, at once, for a status of 400 or
// above (or any, in all mode), and queues the fetch of a text error body.
func logResponse(d *debugLogs, resp playwright.Response) {
	n := d.network
	status := resp.Status()
	if status < 400 && !n.all {
		return
	}
	req := resp.Request()
	r := requestRecord(d, "response", req)
	r.Status, r.StatusText = status, strPtr(resp.StatusText())
	fsw := resp.FromServiceWorker()
	r.FromServiceWorker = &fsw
	headers := resp.Headers() // provisional: local
	r.Headers = allowedHeaders(headers, n.headers)
	if t := req.Timing(); t != nil && t.ResponseStart >= 0 {
		ms := t.ResponseStart
		r.TTFBMs = &ms
	}
	r.ID = n.nextID.Add(1)

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.all {
		n.pending[req] = r.ID
	}
	if status < 400 {
		writeRecord(n.file, r)
		return
	}
	ct := headers["content-type"]
	queue := false
	switch {
	case !isTextContentType(ct):
		r.BodyOmitted = "content-type"
	case !n.accepting:
		r.BodySkipped = "closing"
	default:
		select {
		case n.slots <- struct{}{}:
			queue = true
		default:
			r.BodySkipped = "queue full"
		}
	}
	// The response is on disk before its body is fetched: if the process
	// dies first, the status line is kept.
	writeRecord(n.file, r)
	if queue {
		n.jobs <- bodyJob{id: r.ID, resp: resp, contentType: ct} // never blocks: a slot is held
	}
}

// logRequestEnd writes a requestfailed or (all mode) finished record, with
// the id of the request's response record when there was one.
func logRequestEnd(d *debugLogs, kind string, req playwright.Request) {
	n := d.network
	r := requestRecord(d, kind, req)
	if kind == "requestfailed" {
		failure := ""
		if err := req.Failure(); err != nil {
			failure = err.Error()
		}
		aborted := isAborted(failure)
		r.Failure, r.Aborted = &failure, &aborted
	}
	if t := req.Timing(); n.all && t != nil && t.ResponseEnd >= 0 {
		ms := t.ResponseEnd
		r.DurationMs = &ms
	}
	n.mu.Lock()
	id, ok := n.pending[req]
	delete(n.pending, req)
	n.mu.Unlock()
	if !ok {
		id = n.nextID.Add(1)
	}
	r.ID = id
	if kind == "finished" { // the response record already has the request's details
		r.RequestBody = nil
	}
	writeRecord(n.file, r)
}

// runBodyWorker fetches the queued error bodies, one at a time, off the
// dispatch goroutine (Body is a driver round trip), until Close drains it.
func runBodyWorker(n *netLog) {
	defer close(n.done)
	for j := range n.jobs {
		writeRecord(n.file, fetchBody(n, j))
		<-n.slots
	}
}

// fetchBody returns the responsebody record for j: the start of the body, or
// why there is none.
func fetchBody(n *netLog, j bodyJob) logRecord {
	r := logRecord{Kind: "responsebody", ID: j.id, ContentType: j.contentType}
	type result struct {
		body []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		b, err := j.resp.Body()
		ch <- result{b, err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			r.Error = "body unavailable: " + res.err.Error()
			return r
		}
		body, cut := truncate(string(res.body), n.bodyCap)
		r.Body, r.Truncated = &body, cut
	case <-time.After(n.bodyTimeout):
		r.Error = "body unavailable: timed out after " + n.bodyTimeout.String()
	}
	return r
}

// drainNetwork stops queueing error bodies and gives those already queued a
// short window to arrive, for Close, before the context goes.
func drainNetwork(d *debugLogs) {
	if d == nil || d.network == nil {
		return
	}
	n := d.network
	n.mu.Lock()
	if n.accepting {
		n.accepting = false
		close(n.jobs)
	}
	n.mu.Unlock()
	select {
	case <-n.done:
	case <-time.After(networkDrainWindow):
	}
}

// requestBody is req's body, capped, when its URL matches one of n's
// patterns; nil otherwise.
func requestBody(n *netLog, req playwright.Request) *string {
	if len(n.bodyURLs) == 0 {
		return nil
	}
	u := req.URL()
	for _, re := range n.bodyURLs {
		if re.MatchString(u) {
			b, err := req.PostDataBuffer() // local: from the initializer
			if err != nil || b == nil {
				return nil
			}
			s, _ := truncate(string(b), n.bodyCap)
			return &s
		}
	}
	return nil
}

// neverHeaders are never recorded, whatever the allowlist says.
var neverHeaders = map[string]bool{
	"authorization": true, "proxy-authorization": true, "cookie": true,
	"set-cookie": true, "x-api-key": true, "x-auth-token": true,
}

// allowedHeaders keeps the headers a debug log may hold: the default
// allowlist plus extra, minus neverHeaders. nil when none are left.
func allowedHeaders(h map[string]string, extra map[string]bool) map[string]string {
	var out map[string]string
	for k, v := range h {
		name := strings.ToLower(k)
		if neverHeaders[name] {
			continue
		}
		switch {
		case name == "content-type", name == "content-length", name == "location",
			strings.Contains(name, "trace"), strings.Contains(name, "request-id"),
			strings.Contains(name, "correlation"), extra[name]:
		default:
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = v
	}
	return out
}

// isTextContentType reports whether a body of content type ct is text worth
// recording: text/*, JSON and XML, including the +json and +xml types.
func isTextContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	typ, sub, _ := strings.Cut(mt, "/")
	switch {
	case typ == "text":
		return true
	case typ != "application":
		return false
	}
	return sub == "json" || sub == "xml" || strings.HasSuffix(sub, "+json") || strings.HasSuffix(sub, "+xml")
}

// isAborted reports whether a failure text means the request was cancelled
// (usually by a navigation), in any engine's wording.
func isAborted(failure string) bool {
	return strings.Contains(failure, "ERR_ABORTED") || strings.Contains(failure, "NS_BINDING_ABORTED") ||
		failure == "cancelled"
}

// validateNetworkLog checks the network log options.
func validateNetworkLog(opts Opts) []error {
	var errs []error
	n := opts.Network
	if opts.NetworkLog != "" && !filepath.IsAbs(opts.NetworkLog) {
		errs = append(errs, fmt.Errorf("NetworkLog %q must be an absolute path", opts.NetworkLog))
	}
	switch n.Mode {
	case "", NetworkLogFailures, NetworkLogAll:
	default:
		errs = append(errs, fmt.Errorf("Network.Mode %q must be %q or %q", n.Mode, NetworkLogFailures, NetworkLogAll))
	}
	for name, v := range map[string]int64{
		"BodyCap": int64(n.BodyCap), "BodyTimeout": int64(n.BodyTimeout), "QueueSize": int64(n.QueueSize),
		"FieldCap": int64(n.FieldCap), "MaxBytes": n.MaxBytes,
	} {
		if v < 0 {
			errs = append(errs, fmt.Errorf("Network.%s must not be negative", name))
		}
	}
	for _, p := range n.RequestBodyURLs {
		if _, err := regexp.Compile(p); err != nil {
			errs = append(errs, fmt.Errorf("Network.RequestBodyURLs: %w", err))
		}
	}
	return errs
}
