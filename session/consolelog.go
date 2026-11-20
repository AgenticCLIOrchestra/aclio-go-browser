package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// Console log caps used when Opts.ConsoleLogFieldCap / ConsoleLogMaxBytes
// are 0.
const (
	DefaultConsoleLogFieldCap = 16 << 10 // bytes per text, message or stack
	DefaultConsoleLogMaxBytes = 50 << 20 // bytes per file
)

// stampLayout is the root package's debug dump stamp ({stamp}-{name}-{kind},
// local time, millisecond precision), so a record lines up with the dumps.
const stampLayout = "20060102-150405.000"

// consoleLog writes the browser's console messages, page errors, crashes and
// tab events to a JSON Lines file as they happen (see Opts.ConsoleLog).
//
// Each record is one Write on an O_APPEND file, with no buffering, so it
// reaches the kernel before the handler returns and survives the process
// being killed. The handlers run on playwright-go's dispatch goroutine, so
// they only use what the event already carries (no driver round trips) and
// never touch *Browser; the logger keeps its own state behind mu.
type consoleLog struct {
	mu       sync.Mutex
	f        *os.File // nil once logging has stopped
	size     int64    // bytes written so far
	maxBytes int64
	fieldCap int
	dropped  int                        // records not written past maxBytes
	tabs     map[playwright.Page]string // stable ids, "t1", "t2", …
}

// logSource is where a console message or page error came from. Line and
// column are 0-based, as Playwright reports them.
type logSource struct {
	URL    string `json:"url"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// logRecord is one line of the console log; kind says which fields are set.
type logRecord struct {
	Time        string     `json:"time"`
	Stamp       string     `json:"stamp"`
	Kind        string     `json:"kind"`
	Engine      string     `json:"engine,omitempty"`
	Tab         string     `json:"tab,omitempty"`
	Page        *string    `json:"page,omitempty"`
	Turn        int        `json:"turn,omitempty"`
	Level       string     `json:"level,omitempty"`
	Text        *string    `json:"text,omitempty"`
	Name        *string    `json:"name,omitempty"`
	Message     *string    `json:"message,omitempty"`
	Stack       *string    `json:"stack,omitempty"`
	Source      *logSource `json:"source,omitempty"`
	BrowserTime *float64   `json:"browserTime,omitempty"`
	Worker      *bool      `json:"worker,omitempty"`
	WorkerURL   string     `json:"workerURL,omitempty"`
	Truncated   bool       `json:"truncated,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Dropped     *int       `json:"dropped,omitempty"`
}

// startConsoleLog opens opts.ConsoleLog and writes the open record. Like the
// other debug dumps it is best-effort: on failure it returns nil and the
// session runs without a log.
func startConsoleLog(opts Opts, eng engine.Browser) *consoleLog {
	if opts.ConsoleLog == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.ConsoleLog), 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(opts.ConsoleLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	l := &consoleLog{
		f:        f,
		maxBytes: opts.ConsoleLogMaxBytes,
		fieldCap: opts.ConsoleLogFieldCap,
		tabs:     map[playwright.Page]string{},
	}
	if l.maxBytes == 0 {
		l.maxBytes = DefaultConsoleLogMaxBytes
	}
	if l.fieldCap == 0 {
		l.fieldCap = DefaultConsoleLogFieldCap
	}
	if fi, err := f.Stat(); err == nil {
		l.size = fi.Size()
	}
	logRecordOf(l, logRecord{Kind: "open", Engine: string(eng)})
	return l
}

// listenConsole subscribes l to ctx: console messages (pages, workers and
// service workers), page errors, tabs opening and closing, main-frame
// navigations and crashes, for every tab including those the page opens by
// itself. ctx's existing pages (a persistent context opens with one) are
// recorded too.
func listenConsole(l *consoleLog, ctx playwright.BrowserContext) {
	ctx.OnConsole(func(m playwright.ConsoleMessage) { logConsole(l, m) })
	ctx.OnWebError(func(e playwright.WebError) { logWebError(l, e) })
	ctx.OnPage(func(p playwright.Page) { watchPage(l, p) })
	ctx.OnPageClose(func(p playwright.Page) { logPage(l, "pageclose", p) })
	ctx.OnFrameNavigated(func(f playwright.Frame) {
		if f.ParentFrame() == nil && f.Page() != nil {
			logPage(l, "navigation", f.Page())
		}
	})
	for _, p := range ctx.Pages() {
		watchPage(l, p)
	}
}

// watchPage records p's opening, once, whichever event sees it first.
func watchPage(l *consoleLog, p playwright.Page) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tabLocked(l, p)
}

// logPage writes a kind record about p (pageclose, navigation, crash, active).
func logPage(l *consoleLog, kind string, p playwright.Page) {
	if l == nil || p == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r := logRecord{Kind: kind, Tab: tabLocked(l, p)}
	if kind != "active" {
		r.Page = strPtr(p.URL())
	}
	writeLocked(l, r)
}

func logConsole(l *consoleLog, m playwright.ConsoleMessage) {
	text, cut := truncate(m.Text(), l.fieldCap)
	r := logRecord{Kind: "console", Level: m.Type(), Text: &text, Truncated: cut}
	if loc := m.Location(); loc != nil {
		r.Source = &logSource{URL: loc.URL, Line: loc.Line, Column: loc.Column}
	}
	if ts, err := m.Timestamp(); err == nil && ts != 0 {
		r.BrowserTime = &ts
	}
	w, _ := m.Worker() // local: the worker the event names
	isWorker := w != nil
	r.Worker = &isWorker
	if isWorker {
		r.WorkerURL = w.URL()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := m.Page(); p != nil {
		r.Tab, r.Page = tabLocked(l, p), strPtr(p.URL())
	}
	writeLocked(l, r)
}

func logWebError(l *consoleLog, e playwright.WebError) {
	r := logRecord{Kind: "pageerror"}
	var pe *playwright.Error
	switch err := e.Error(); {
	case errors.As(err, &pe):
		name, nc := truncate(pe.Name, l.fieldCap)
		msg, mc := truncate(pe.Message, l.fieldCap)
		stack, sc := truncate(pe.Stack, l.fieldCap)
		r.Name, r.Message, r.Stack, r.Truncated = &name, &msg, &stack, nc || mc || sc
	case err != nil:
		msg, mc := truncate(err.Error(), l.fieldCap)
		r.Message, r.Truncated = &msg, mc
	}
	if loc := e.Location(); loc != nil {
		r.Source = &logSource{URL: loc.URL, Line: loc.Line, Column: loc.Column}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := e.Page(); p != nil {
		r.Tab, r.Page = tabLocked(l, p), strPtr(p.URL())
	}
	writeLocked(l, r)
}

// tabLocked returns p's stable id. The first time p is seen it gives it the
// next one, writes its pageopen record and listens for its crash, which
// Playwright only reports per page. (Adding a listener is local: no driver
// round trip.)
func tabLocked(l *consoleLog, p playwright.Page) string {
	if id, ok := l.tabs[p]; ok {
		return id
	}
	id := "t" + strconv.Itoa(len(l.tabs)+1)
	l.tabs[p] = id
	writeLocked(l, logRecord{Kind: "pageopen", Tab: id, Page: strPtr(p.URL())})
	p.OnCrash(func(p playwright.Page) { logPage(l, "crash", p) })
	return id
}

func logRecordOf(l *consoleLog, r logRecord) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	writeLocked(l, r)
}

// writeLocked writes r as one line, with l.mu held. The first record that
// would go past maxBytes is replaced by a single dropped record; it and every
// record after it, however small, are only counted. A write error stops
// logging.
func writeLocked(l *consoleLog, r logRecord) {
	if l.f == nil {
		return
	}
	if l.dropped > 0 { // the cap was reached: from then on, only count
		l.dropped++
		return
	}
	r = capURLs(r, l.fieldCap)
	line, ok := marshalRecord(r, time.Now())
	if !ok {
		return
	}
	if l.size+int64(len(line)) > l.maxBytes {
		if l.dropped == 0 {
			if d, ok := marshalRecord(logRecord{Kind: "dropped", Reason: "file size cap reached"}, time.Now()); ok {
				writeLine(l, d)
			}
		}
		l.dropped++
		return
	}
	writeLine(l, line)
}

// capURLs cuts r's URLs (page, source and worker) at n bytes, like the text
// fields, so a page on a huge data: URL can't make every record that big.
func capURLs(r logRecord, n int) logRecord {
	var cut bool
	if r.Page != nil {
		p, c := truncate(*r.Page, n)
		r.Page, cut = &p, cut || c
	}
	if r.Source != nil {
		s := *r.Source
		var c bool
		s.URL, c = truncate(s.URL, n)
		r.Source, cut = &s, cut || c
	}
	var c bool
	r.WorkerURL, c = truncate(r.WorkerURL, n)
	r.Truncated = r.Truncated || cut || c
	return r
}

func writeLine(l *consoleLog, line []byte) {
	n, err := l.f.Write(line)
	l.size += int64(n)
	if err != nil {
		_ = l.f.Close()
		l.f = nil
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

// stopConsoleLog writes the closed record, with the number of records
// dropped past the size cap, and closes the file. Nothing is logged after.
func stopConsoleLog(l *consoleLog) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}
	if line, ok := marshalRecord(logRecord{Kind: "closed", Dropped: &l.dropped}, time.Now()); ok {
		writeLine(l, line)
	}
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
}

// LogTurn writes a turn marker to b's console log, so the records of turn n
// are the ones between its marker and the next. It does nothing when b
// keeps no console log (Opts.ConsoleLog).
func LogTurn(b *Browser, n int) {
	if b == nil {
		return
	}
	logRecordOf(b.consoleLog, logRecord{Kind: "turn", Turn: n})
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

// validateConsoleLog checks the console log options.
func validateConsoleLog(opts Opts) []error {
	var errs []error
	if opts.ConsoleLog != "" && !filepath.IsAbs(opts.ConsoleLog) {
		errs = append(errs, fmt.Errorf("ConsoleLog %q must be an absolute path", opts.ConsoleLog))
	}
	if opts.ConsoleLogFieldCap < 0 {
		errs = append(errs, fmt.Errorf("ConsoleLogFieldCap %d must not be negative", opts.ConsoleLogFieldCap))
	}
	if opts.ConsoleLogMaxBytes < 0 {
		errs = append(errs, fmt.Errorf("ConsoleLogMaxBytes %d must not be negative", opts.ConsoleLogMaxBytes))
	}
	return errs
}
