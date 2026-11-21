package session

import (
	"fmt"
	"sync"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/mxschmitt/playwright-go"
)

// announcer records what a screen reader says as it happens, so that every
// focus change and every change to the focused element is heard, not just
// the state the page ends a turn in. The page queues the element focused at
// each such event (announcerJS); Go turns each into an announcement with
// preset.Announce, against the one before. The queue is processed in the
// background when the page signals, after each action Perform runs, and when
// the announcements are taken.
type announcer struct {
	mu   sync.Mutex
	lang preset.Language
	page playwright.Page        // the active tab, whose queue is read
	cdp  playwright.CDPSession  // on page
	last *preset.FocusedElement // the element announced last, nil after focus was lost
	said []string               // announced since the last TakeAnnouncements

	signal chan struct{} // the page has queued something
	done   chan struct{} // closed by Close
}

// announcerSignal is the function the page calls to wake the background
// processing.
const announcerSignal = "__aclioAnnouncerSignal"

// announcerJS installs the page side: it queues the focused element on every
// focusin, on input / change events of the focused element, and on changes
// to the attributes of the focused element or its active descendant that a
// screen reader announces; it queues the document when focus is lost. It
// runs in every new document (an init script) and in the active tab's
// current one.
const announcerJS = `(() => {
  if (window.__aclioAnnouncer) return;
  const MAX = 1000;
  const q = [];
  const signal = () => {
    try {
      const p = window.` + announcerSignal + ` && window.` + announcerSignal + `();
      if (p && p.catch) p.catch(() => {});
    } catch (e) {}
  };
  const push = (x) => { if (q.length >= MAX) q.shift(); q.push(x); signal(); };
  const focused = () => {
    const el = document.activeElement;
    return el && el !== document.body && el !== document.documentElement ? el : null;
  };
  const record = () => { const el = focused(); if (el) push(el); };
  document.addEventListener('focusin', record, true);
  document.addEventListener('focusout', (e) => {
    if (!e.relatedTarget) setTimeout(() => { if (!focused()) push(document); }, 0);
  }, true);
  for (const type of ['input', 'change']) {
    document.addEventListener(type, (e) => { if (e.target === document.activeElement) record(); }, true);
  }
  const WATCH = ['aria-checked', 'aria-pressed', 'aria-selected', 'aria-expanded', 'aria-disabled',
    'aria-invalid', 'aria-required', 'aria-readonly', 'aria-activedescendant', 'aria-valuenow',
    'aria-valuetext', 'aria-label', 'aria-labelledby', 'aria-describedby', 'aria-description',
    'aria-hidden', 'aria-level', 'aria-posinset', 'aria-setsize', 'disabled', 'checked',
    'selected', 'value', 'required', 'readonly', 'hidden', 'open', 'title'];
  new MutationObserver((ms) => {
    const el = focused();
    if (!el) return;
    const adId = el.getAttribute('aria-activedescendant');
    if (ms.some((m) => m.target === el || (adId && m.target.id === adId))) record();
  }).observe(document, { attributes: true, subtree: true, attributeFilter: WATCH });
  window.__aclioAnnouncer = { next: () => q.shift(), now: record };
})()`

// announcerGroup is the CDP object group the announcer's element references
// are held in, released after each processing.
const announcerGroup = "aclio-announcer"

// StartAnnouncing makes b record what a screen reader says, in lang (the zero
// value meaning preset.DefaultLanguage), as it happens: every focus change,
// and every change to the focused element or its active descendant, in the
// active tab, including those the page's own scripts make. Take what was said
// with TakeAnnouncements. The element focused when it starts is announced
// first. Calling it again only changes the language. It needs Chromium, like
// Focused.
func StartAnnouncing(b *Browser, lang preset.Language) error {
	if err := checkOpen(b); err != nil {
		return err
	}
	if b.engine != engine.Chromium {
		return fmt.Errorf("session: announcing needs Chromium, not %s", b.engine)
	}
	lang, err := preset.ResolveLanguage(lang)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if a := b.announcer; a != nil {
		a.mu.Lock()
		a.lang = lang
		a.mu.Unlock()
		return nil
	}
	a := &announcer{lang: lang, signal: make(chan struct{}, 1), done: make(chan struct{})}
	script := announcerJS
	if err := b.context.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("session: installing the announcer: %w", err)
	}
	if err := b.context.ExposeBinding(announcerSignal, func(*playwright.BindingSource, ...any) any {
		select {
		case a.signal <- struct{}{}:
		default: // a wake-up is already pending
		}
		return nil
	}); err != nil {
		return fmt.Errorf("session: installing the announcer: %w", err)
	}
	if err := attachAnnouncer(a, b.active); err != nil {
		return err
	}
	b.announcer = a
	go runAnnouncer(a)
	return nil
}

// TakeAnnouncements returns everything the screen reader has said since the
// last call (or since StartAnnouncing), one spoken item per string, in order,
// and forgets it. It is empty when nothing was said, and nil when b isn't
// announcing.
func TakeAnnouncements(b *Browser) []string {
	if b == nil || b.announcer == nil {
		return nil
	}
	a := b.announcer
	a.mu.Lock()
	defer a.mu.Unlock()
	processLocked(a)
	said := a.said
	if said == nil {
		said = []string{}
	}
	a.said = nil
	return said
}

// runAnnouncer processes the queue whenever the page signals, until Close.
func runAnnouncer(a *announcer) {
	for {
		select {
		case <-a.done:
			return
		case <-a.signal:
			processAnnouncements(a)
		}
	}
}

// processAnnouncements turns what the page has queued into announcements.
func processAnnouncements(a *announcer) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	processLocked(a)
}

// processLocked does processAnnouncements with a.mu held. Failures (a page
// mid-navigation, a closed tab) end the round; what is still queued is read
// next time, and a queue lost with its document is lost with it.
func processLocked(a *announcer) {
	if a.cdp == nil {
		return
	}
	defer a.cdp.Send("Runtime.releaseObjectGroup", map[string]any{"objectGroup": announcerGroup}) //nolint:errcheck // best-effort cleanup
	for range 1000 {
		var ev struct {
			Result struct {
				Type      string `json:"type"`
				ClassName string `json:"className"`
				ObjectID  string `json:"objectId"`
			} `json:"result"`
		}
		if err := cdpSend(a.cdp, "Runtime.evaluate", map[string]any{
			"expression":  `window.__aclioAnnouncer ? window.__aclioAnnouncer.next() : undefined`,
			"objectGroup": announcerGroup,
		}, &ev); err != nil {
			return
		}
		switch {
		case ev.Result.Type == "undefined" || ev.Result.ObjectID == "":
			return
		case ev.Result.ClassName == "HTMLDocument": // focus was lost
			a.last = nil
			continue
		}
		fe, err := focusedOf(a.cdp, ev.Result.ObjectID)
		if err != nil || fe == nil {
			continue
		}
		a.said = append(a.said, preset.Announce(fe, a.last, a.lang)...)
		a.last = fe
	}
}

// attachAnnouncer points a at page p, the new active tab: its own queue from
// now on, the page side installed in its current document if missing, and
// its focused element queued, so a screen reader's "where am I" after the
// switch is heard.
func attachAnnouncer(a *announcer, p playwright.Page) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cdp != nil {
		a.cdp.Detach() //nolint:errcheck // the old tab may already be closed
		a.cdp = nil
	}
	cdp, err := p.Context().NewCDPSession(p)
	if err != nil {
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	if _, err := p.Evaluate(announcerJS); err != nil {
		cdp.Detach() //nolint:errcheck
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	if _, err := p.Evaluate(`() => window.__aclioAnnouncer.now()`); err != nil {
		cdp.Detach() //nolint:errcheck
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	a.page, a.cdp, a.last = p, cdp, nil
	return nil
}

// setActive makes p b's active tab, records it in the debug logs, and makes
// it the announcer's, when b announces. An announcer that can't follow (rare:
// the tab closing under it) stays quiet rather than failing the tab action.
func setActive(b *Browser, p playwright.Page) {
	b.active = p
	logTab(b.logs, "active", p)
	if b.announcer != nil {
		_ = attachAnnouncer(b.announcer, p)
	}
}

// stopAnnouncer ends the background processing, for Close.
func stopAnnouncer(b *Browser) {
	a := b.announcer
	if a == nil {
		return
	}
	close(a.done)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cdp != nil {
		a.cdp.Detach() //nolint:errcheck
		a.cdp = nil
	}
}
