package session

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// Debug log caps used when a log's field cap or file cap is 0.
const (
	DefaultConsoleLogFieldCap = 16 << 10 // bytes per text, message, stack or URL
	DefaultConsoleLogMaxBytes = 50 << 20 // bytes per file
)

// stampLayout is the root package's debug dump stamp ({stamp}-{name}-{kind},
// local time, millisecond precision), so a record lines up with the dumps.
const stampLayout = "20060102-150405.000"

// debugLogs are the debug logs a session keeps: the console log
// (Opts.ConsoleLog) and the network log (Opts.NetworkLog), either of which
// may be off. They share the tab ids, the tab records (pageopen, pageclose,
// navigation, active, crash) and the turn markers, so both files read the
// same way.
//
// Every record is one Write on an O_APPEND file, with no buffering, so it
// reaches the kernel before the handler returns and survives the process
// being killed. The handlers run on playwright-go's dispatch goroutine, so
// they only use what the event already carries (no driver round trips) and
// never touch *Browser; the logs keep their own state behind their own
// mutexes. Lock order: netLog.mu, then tabsMu, then a logFile's mu.
type debugLogs struct {
	tabsMu sync.Mutex
	tabs   map[playwright.Page]string // stable ids, "t1", "t2", …

	console *logFile // nil when off
	network *netLog  // nil when off
}

// logFile is one JSON Lines file.
type logFile struct {
	mu       sync.Mutex
	f        *os.File // nil once logging has stopped
	size     int64    // bytes written so far
	maxBytes int64
	fieldCap int
	dropped  int // records not written past maxBytes
}

// logSource is where a console message or page error came from. Line and
// column are 0-based, as Playwright reports them.
type logSource struct {
	URL    string `json:"url"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// logRecord is one line of a debug log; kind says which fields are set.
type logRecord struct {
	Time  string `json:"time"`
	Stamp string `json:"stamp"`
	Kind  string `json:"kind"`
	ID    int64  `json:"id,omitempty"`

	Engine string  `json:"engine,omitempty"`
	Tab    string  `json:"tab,omitempty"`
	Page   *string `json:"page,omitempty"`
	Turn   int     `json:"turn,omitempty"`

	// console and pageerror
	Level       string     `json:"level,omitempty"`
	Text        *string    `json:"text,omitempty"`
	Name        *string    `json:"name,omitempty"`
	Message     *string    `json:"message,omitempty"`
	Stack       *string    `json:"stack,omitempty"`
	Source      *logSource `json:"source,omitempty"`
	BrowserTime *float64   `json:"browserTime,omitempty"`
	Worker      *bool      `json:"worker,omitempty"`
	WorkerURL   string     `json:"workerURL,omitempty"`

	// network
	Method            string            `json:"method,omitempty"`
	URL               *string           `json:"url,omitempty"`
	ResourceType      string            `json:"resourceType,omitempty"`
	Status            int               `json:"status,omitempty"`
	StatusText        *string           `json:"statusText,omitempty"`
	FromServiceWorker *bool             `json:"fromServiceWorker,omitempty"`
	TTFBMs            *float64          `json:"ttfbMs,omitempty"`
	DurationMs        *float64          `json:"durationMs,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	RequestBody       *string           `json:"requestBody,omitempty"`
	BodyOmitted       string            `json:"bodyOmitted,omitempty"`
	BodySkipped       string            `json:"bodySkipped,omitempty"`
	ContentType       string            `json:"contentType,omitempty"`
	Body              *string           `json:"body,omitempty"`
	Failure           *string           `json:"failure,omitempty"`
	Aborted           *bool             `json:"aborted,omitempty"`
	Error             string            `json:"error,omitempty"`

	Truncated bool   `json:"truncated,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Dropped   *int   `json:"dropped,omitempty"`
	Pending   *int   `json:"pending,omitempty"`
}

// startLogs opens the logs opts asks for and writes their open records. Like
// the other debug dumps they are best-effort: one that can't be opened is
// off, and nil means both are.
func startLogs(opts Opts, eng engine.Browser) *debugLogs {
	d := &debugLogs{tabs: map[playwright.Page]string{}}
	d.console = openLogFile(opts.ConsoleLog, opts.ConsoleLogFieldCap, opts.ConsoleLogMaxBytes)
	if f := openLogFile(opts.NetworkLog, opts.Network.FieldCap, opts.Network.MaxBytes); f != nil {
		d.network = newNetLog(f, opts.Network)
	}
	if d.console == nil && d.network == nil {
		return nil
	}
	writeAll(d, logRecord{Kind: "open", Engine: string(eng)})
	return d
}

func openLogFile(path string, fieldCap int, maxBytes int64) *logFile {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	lf := &logFile{f: f, maxBytes: maxBytes, fieldCap: fieldCap}
	if lf.maxBytes == 0 {
		lf.maxBytes = DefaultConsoleLogMaxBytes
	}
	if lf.fieldCap == 0 {
		lf.fieldCap = DefaultConsoleLogFieldCap
	}
	if fi, err := f.Stat(); err == nil {
		lf.size = fi.Size()
	}
	return lf
}

// listenLogs subscribes d to ctx: the tab events both logs share, and each
// log's own events. ctx's existing pages (a persistent context opens with
// one) are recorded too.
func listenLogs(d *debugLogs, ctx playwright.BrowserContext) {
	ctx.OnPage(func(p playwright.Page) { watchPage(d, p) })
	ctx.OnPageClose(func(p playwright.Page) { logTab(d, "pageclose", p) })
	ctx.OnFrameNavigated(func(f playwright.Frame) {
		if f.ParentFrame() == nil && f.Page() != nil {
			logTab(d, "navigation", f.Page())
		}
	})
	if d.console != nil {
		listenConsole(d, ctx)
	}
	if d.network != nil {
		listenNetwork(d, ctx)
	}
	for _, p := range ctx.Pages() {
		watchPage(d, p)
	}
}

// watchPage records p's opening, once, whichever event sees it first.
func watchPage(d *debugLogs, p playwright.Page) {
	tabID(d, p)
}

// tabID returns p's stable id. The first time p is seen it gives it the next
// one, writes its pageopen record to both logs and listens for its crash,
// which Playwright only reports per page. (Adding a listener is local: no
// driver round trip.)
func tabID(d *debugLogs, p playwright.Page) string {
	d.tabsMu.Lock()
	defer d.tabsMu.Unlock()
	if id, ok := d.tabs[p]; ok {
		return id
	}
	id := "t" + strconv.Itoa(len(d.tabs)+1)
	d.tabs[p] = id
	writeAll(d, logRecord{Kind: "pageopen", Tab: id, Page: strPtr(p.URL())})
	p.OnCrash(func(p playwright.Page) { logTab(d, "crash", p) })
	return id
}

// pageOf fills in r's tab and page for p, when there is one.
func pageOf(d *debugLogs, r *logRecord, p playwright.Page) {
	if p != nil {
		r.Tab, r.Page = tabID(d, p), strPtr(p.URL())
	}
}

// logTab writes a kind record about p (pageclose, navigation, crash, active)
// to both logs.
func logTab(d *debugLogs, kind string, p playwright.Page) {
	if d == nil || p == nil {
		return
	}
	r := logRecord{Kind: kind, Tab: tabID(d, p)}
	if kind != "active" {
		r.Page = strPtr(p.URL())
	}
	writeAll(d, r)
}

// LogTurn writes a turn marker to b's debug logs (Opts.ConsoleLog and
// Opts.NetworkLog), so the records of turn n are the ones between its marker
// and the next. It does nothing when b keeps neither.
func LogTurn(b *Browser, n int) {
	if b == nil {
		return
	}
	writeAll(b.logs, logRecord{Kind: "turn", Turn: n})
}

// writeAll writes r to every log d keeps.
func writeAll(d *debugLogs, r logRecord) {
	if d == nil {
		return
	}
	writeRecord(d.console, r)
	if d.network != nil {
		writeRecord(d.network.file, r)
	}
}

// writeRecord writes r as one line of lf (nil: off). Its URLs are redacted
// (redactURL) and, like the other text fields, cut at the field cap. The
// first record that would go past maxBytes is replaced by a single dropped
// record; it and every record after it, however small, are only counted. A
// write error stops logging.
func writeRecord(lf *logFile, r logRecord) {
	if lf == nil {
		return
	}
	lf.mu.Lock()
	defer lf.mu.Unlock()
	if lf.f == nil {
		return
	}
	if lf.dropped > 0 { // the cap was reached: from then on, only count
		lf.dropped++
		return
	}
	line, ok := marshalRecord(sanitizeURLs(r, lf.fieldCap), time.Now())
	if !ok {
		return
	}
	if lf.size+int64(len(line)) > lf.maxBytes {
		if d, ok := marshalRecord(logRecord{Kind: "dropped", Reason: "file size cap reached"}, time.Now()); ok {
			writeLine(lf, d)
		}
		lf.dropped++
		return
	}
	writeLine(lf, line)
}

// sanitizeURLs redacts r's URLs (page, url, source and worker) and cuts them
// at n bytes, so neither a token in a query string nor a page on a huge
// data: URL ends up in the file.
func sanitizeURLs(r logRecord, n int) logRecord {
	var cut bool
	fix := func(s string) string {
		s, c := truncate(redactURL(s), n)
		cut = cut || c
		return s
	}
	if r.Page != nil {
		r.Page = strPtr(fix(*r.Page))
	}
	if r.URL != nil {
		r.URL = strPtr(fix(*r.URL))
	}
	if r.Source != nil {
		s := *r.Source
		s.URL = fix(s.URL)
		r.Source = &s
	}
	if r.WorkerURL != "" {
		r.WorkerURL = fix(r.WorkerURL)
	}
	r.Truncated = r.Truncated || cut
	return r
}

func writeLine(lf *logFile, line []byte) {
	n, err := lf.f.Write(line)
	lf.size += int64(n)
	if err != nil {
		_ = lf.f.Close()
		lf.f = nil
	}
}

func marshalRecord(r logRecord, now time.Time) ([]byte, bool) {
	r.Time = now.UTC().Format(time.RFC3339Nano)
	r.Stamp = now.Format(stampLayout)
	b, err := json.Marshal(r)
	if err != nil {
		return nil, false
	}
	return append(b, '\n'), true
}

// stopLogs writes each log's closed record, with the number of records
// dropped past the size cap (and, for the network log, the response bodies
// still being fetched), and closes the files. Nothing is logged after.
func stopLogs(d *debugLogs) {
	if d == nil {
		return
	}
	closeLogFile(d.console, nil)
	if n := d.network; n != nil {
		pending := len(n.slots)
		closeLogFile(n.file, &pending)
		n.mu.Lock()
		clear(n.pending)
		n.mu.Unlock()
	}
}

func closeLogFile(lf *logFile, pending *int) {
	if lf == nil {
		return
	}
	lf.mu.Lock()
	defer lf.mu.Unlock()
	if lf.f == nil {
		return
	}
	if line, ok := marshalRecord(logRecord{Kind: "closed", Dropped: &lf.dropped, Pending: pending}, time.Now()); ok {
		writeLine(lf, line)
	}
	if lf.f != nil {
		_ = lf.f.Close()
		lf.f = nil
	}
}

// sensitiveParams are the query and fragment parameters whose values
// redactURL replaces; so is any parameter whose name contains "token" or
// "secret".
var sensitiveParams = map[string]bool{
	"code": true, "state": true, "token": true, "access_token": true, "id_token": true,
	"refresh_token": true, "password": true, "passwd": true, "secret": true,
	"client_secret": true, "key": true, "api_key": true, "apikey": true,
	"signature": true, "sig": true, "session": true,
}

// redactURL replaces the values of sensitive parameters (sensitiveParams) in
// s's query string and fragment with REDACTED, leaving the rest of s as it
// was. A fragment that is a client-side route ("#/login?code=…") has the
// parameters after its "?" checked.
func redactURL(s string) string {
	base, frag, hasFrag := strings.Cut(s, "#")
	if path, query, ok := strings.Cut(base, "?"); ok {
		base = path + "?" + redactParams(query)
	}
	if !hasFrag {
		return base
	}
	if route, query, ok := strings.Cut(frag, "?"); ok {
		frag = route + "?" + redactParams(query)
	} else {
		frag = redactParams(frag)
	}
	return base + "#" + frag
}

// redactParams redacts the sensitive values of a k=v&k=v list.
func redactParams(q string) string {
	parts := strings.Split(q, "&")
	for i, p := range parts {
		name, _, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		key := name
		if u, err := url.QueryUnescape(name); err == nil {
			key = u
		}
		key = strings.ToLower(key)
		if sensitiveParams[key] || strings.Contains(key, "token") || strings.Contains(key, "secret") {
			parts[i] = name + "=REDACTED"
		}
	}
	return strings.Join(parts, "&")
}

// truncate cuts s to at most n bytes, on a UTF-8 boundary, and reports
// whether it did.
func truncate(s string, n int) (string, bool) {
	if n <= 0 || len(s) <= n {
		return s, false
	}
	i := n
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i], true
}

func strPtr(s string) *string { return &s }
