package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/mxschmitt/playwright-go"
)

// fakePage is a playwright.Page with only what the console log uses.
type fakePage struct {
	playwright.Page
	url     string
	onCrash func(playwright.Page)
}

func (p *fakePage) URL() string                      { return p.url }
func (p *fakePage) OnCrash(fn func(playwright.Page)) { p.onCrash = fn }

func readLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func kinds(recs []map[string]any) []string {
	var out []string
	for _, r := range recs {
		out = append(out, r["kind"].(string))
	}
	return out
}

func TestConsoleLogRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "log.jsonl")
	l := startConsoleLog(Opts{ConsoleLog: path}, engine.Chromium)
	if l == nil {
		t.Fatal("no log")
	}
	p1, p2 := &fakePage{url: "about:blank"}, &fakePage{url: "http://x/2"}
	watchPage(l, p1)
	watchPage(l, p1) // seen once only
	LogTurn(&Browser{consoleLog: l}, 1)
	logPage(l, "navigation", p2) // first seen here: pageopen first
	logPage(l, "active", p2)
	p1.onCrash(p1)
	logPage(l, "pageclose", p2)

	// Flushed as it happens: readable before stop.
	got := strings.Join(kinds(readLog(t, path)), ",")
	if want := "open,pageopen,turn,pageopen,navigation,active,crash,pageclose"; got != want {
		t.Fatalf("kinds = %s, want %s", got, want)
	}
	stopConsoleLog(l)
	recs := readLog(t, path)
	last := recs[len(recs)-1]
	if last["kind"] != "closed" || last["dropped"] != 0.0 {
		t.Errorf("last = %v", last)
	}
	if recs[0]["engine"] != "chromium" {
		t.Errorf("open = %v", recs[0])
	}
	if recs[1]["tab"] != "t1" || recs[3]["tab"] != "t2" || recs[4]["page"] != "http://x/2" {
		t.Errorf("tabs = %v", recs)
	}
	if _, ok := recs[5]["page"]; ok {
		t.Errorf("active has a page: %v", recs[5])
	}
	if recs[2]["turn"] != 1.0 {
		t.Errorf("turn = %v", recs[2])
	}
	for _, r := range recs {
		if _, err := time.Parse(time.RFC3339Nano, r["time"].(string)); err != nil {
			t.Errorf("time: %v", err)
		}
		if _, err := time.ParseInLocation(stampLayout, r["stamp"].(string), time.Local); err != nil {
			t.Errorf("stamp: %v", err)
		}
	}

	// Stopped: nothing more is written, and stopping again is harmless.
	LogTurn(&Browser{consoleLog: l}, 2)
	stopConsoleLog(l)
	if n := len(readLog(t, path)); n != len(recs) {
		t.Errorf("records after stop: %d, want %d", n, len(recs))
	}
}

func TestConsoleLogFileCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l := startConsoleLog(Opts{ConsoleLog: path, ConsoleLogMaxBytes: 1000}, engine.Firefox)
	b := &Browser{consoleLog: l}
	for i := range 100 {
		LogTurn(b, i+1)
	}
	stopConsoleLog(l)
	recs := readLog(t, path)
	dropped := 0
	for _, r := range recs {
		if r["kind"] == "dropped" {
			dropped++
		}
	}
	if dropped != 1 {
		t.Errorf("dropped records = %d, want 1", dropped)
	}
	turns := len(recs) - 3 // open, dropped, closed
	last := recs[len(recs)-1]
	if last["kind"] != "closed" || int(last["dropped"].(float64)) != 100-turns {
		t.Errorf("closed = %v with %d turns written", last, turns)
	}
	fi, _ := os.Stat(path)
	if fi.Size() > 1000+300 { // the cap, plus dropped and closed
		t.Errorf("size = %d", fi.Size())
	}
}

// Once the cap drops a record, everything after it is dropped too, even
// records small enough to fit.
func TestConsoleLogDropsEverythingAfterTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l := startConsoleLog(Opts{ConsoleLog: path, ConsoleLogMaxBytes: 600}, engine.Chromium)
	logPage(l, "navigation", &fakePage{url: "http://x/" + strings.Repeat("a", 1000)}) // too big: pageopen dropped
	LogTurn(&Browser{consoleLog: l}, 1)                                               // would fit, but comes after a drop
	stopConsoleLog(l)
	recs := readLog(t, path)
	if got := strings.Join(kinds(recs), ","); got != "open,dropped,closed" {
		t.Fatalf("kinds = %s", got)
	}
	if d := recs[2]["dropped"]; d != 3.0 { // pageopen, navigation, turn
		t.Errorf("dropped = %v, want 3", d)
	}
}

func TestConsoleLogCapsURLs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l := startConsoleLog(Opts{ConsoleLog: path, ConsoleLogFieldCap: 20}, engine.Chromium)
	huge := "data:text/html," + strings.Repeat("x", 10000)
	logPage(l, "navigation", &fakePage{url: huge})
	logRecordOf(l, logRecord{Kind: "console", Source: &logSource{URL: huge}, WorkerURL: huge})
	stopConsoleLog(l)
	for _, r := range readLog(t, path)[1:3] { // pageopen, navigation
		if p := r["page"].(string); len(p) != 20 || r["truncated"] != true {
			t.Errorf("%v: page not capped", r["kind"])
		}
	}
	r := readLog(t, path)[3]
	if len(r["source"].(map[string]any)["url"].(string)) != 20 || len(r["workerURL"].(string)) != 20 || r["truncated"] != true {
		t.Errorf("console = %v", r)
	}
}

func TestConsoleLogOff(t *testing.T) {
	if l := startConsoleLog(Opts{}, engine.Chromium); l != nil {
		t.Error("log without a path")
	}
	// An unwritable location turns logging off instead of failing.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if l := startConsoleLog(Opts{ConsoleLog: filepath.Join(file, "log.jsonl")}, engine.Chromium); l != nil {
		t.Error("log under a file")
	}
	// Every entry point accepts no log.
	LogTurn(nil, 1)
	LogTurn(&Browser{}, 1)
	logPage(nil, "active", &fakePage{})
	stopConsoleLog(nil)
}

func TestTruncate(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
		cut  bool
	}{
		{"hello", 10, "hello", false},
		{"hello", 5, "hello", false},
		{"hello", 3, "hel", true},
		{"héllo", 2, "h", true}, // é is 2 bytes: not split
		{"héllo", 3, "hé", true},
	} {
		got, cut := truncate(c.in, c.n)
		if got != c.want || cut != c.cut {
			t.Errorf("truncate(%q, %d) = %q, %v; want %q, %v", c.in, c.n, got, cut, c.want, c.cut)
		}
	}
}

func TestValidateConsoleLog(t *testing.T) {
	errs := validateConsoleLog(Opts{ConsoleLog: "rel.jsonl", ConsoleLogFieldCap: -1, ConsoleLogMaxBytes: -1})
	if len(errs) != 3 {
		t.Errorf("errs = %v", errs)
	}
	if errs := validateConsoleLog(Opts{ConsoleLog: "/abs/log.jsonl"}); len(errs) != 0 {
		t.Errorf("errs = %v", errs)
	}
}
