package session

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mxschmitt/playwright-go"
)

// listenConsole subscribes d's console log to ctx: console messages (pages,
// workers and service workers) and uncaught page errors, for every tab
// including those the page opens by itself.
func listenConsole(d *debugLogs, ctx playwright.BrowserContext) {
	ctx.OnConsole(func(m playwright.ConsoleMessage) { logConsole(d, m) })
	ctx.OnWebError(func(e playwright.WebError) { logWebError(d, e) })
}

func logConsole(d *debugLogs, m playwright.ConsoleMessage) {
	text, cut := truncate(m.Text(), d.console.fieldCap)
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
	pageOf(d, &r, m.Page())
	writeRecord(d.console, r)
}

func logWebError(d *debugLogs, e playwright.WebError) {
	fieldCap := d.console.fieldCap
	r := logRecord{Kind: "pageerror"}
	var pe *playwright.Error
	switch err := e.Error(); {
	case errors.As(err, &pe):
		name, nc := truncate(pe.Name, fieldCap)
		msg, mc := truncate(pe.Message, fieldCap)
		stack, sc := truncate(pe.Stack, fieldCap)
		r.Name, r.Message, r.Stack, r.Truncated = &name, &msg, &stack, nc || mc || sc
	case err != nil:
		msg, mc := truncate(err.Error(), fieldCap)
		r.Message, r.Truncated = &msg, mc
	}
	if loc := e.Location(); loc != nil {
		r.Source = &logSource{URL: loc.URL, Line: loc.Line, Column: loc.Column}
	}
	pageOf(d, &r, e.Page())
	writeRecord(d.console, r)
}

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
