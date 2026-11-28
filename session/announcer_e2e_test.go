package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// liveServer serves the live region test pages: /blank (for the page-side
// rules), /form (a focus-only form), /search (a combobox shaped like a
// design system's search: a status region after the input, updated from a
// debounced search), /select (react-select's markup: an aria-live log with
// its own spans), /frame (a region in an iframe) and /alert (an alert shown
// on a button press).
func liveServer(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/blank": `<!doctype html><title>Blank</title><body></body>`,
		"/form": `<!doctype html><title>Form</title><main><form aria-label="Prefs">
<label for="a">First</label><input id="a">
<label for="b">Second</label><input id="b">
<button type="button">Save</button></form></main>`,
		"/search": `<!doctype html><title>Search</title>
<label for="r">Recipients</label><input id="r" role="combobox" aria-expanded="false" aria-autocomplete="list">
<div role="status" id="results"></div>
<script>
const users = ['Ada', 'Alan', 'Grace'];
let timer;
document.getElementById('r').addEventListener('input', (e) => {
  clearTimeout(timer);
  timer = setTimeout(() => {
    const n = users.filter((u) => u.toLowerCase().startsWith(e.target.value.toLowerCase())).length;
    document.getElementById('results').textContent = n ? n + ' results available' : 'No matching users or groups.';
  }, 100);
});
</script>`,
		"/select": `<!doctype html><title>Select</title>
<span id="aria-live-region" aria-live="polite" aria-atomic="false" aria-relevant="additions text" role="log"></span>
<label for="s">Colour</label><input id="s" role="combobox" aria-expanded="false">
<script>
document.getElementById('s').addEventListener('input', (e) => {
  const log = document.getElementById('aria-live-region');
  log.replaceChildren();
  const sel = document.createElement('span'); sel.id = 'aria-selection';
  const res = document.createElement('span'); res.id = 'aria-results';
  res.textContent = e.target.value === 'r' ? '1 result available.' : '3 results available.';
  const guide = document.createElement('span'); guide.id = 'aria-guidance';
  guide.textContent = 'Use Up and Down to choose options.';
  log.append(sel, res, guide);
});
</script>`,
		"/chat": `<!doctype html><title>Chat</title><button id="ask">Ask</button>
<div role="log" aria-relevant="additions" id="log"><p>Q</p><p>A</p></div>
<script>
// Remounts every message, as a React re-render that keys them afresh does.
window.rerender = () => { const log = document.getElementById('log'); log.innerHTML = Array.from(log.children, (p) => '<p>' + p.textContent + '</p>').join(''); };
</script>`,
		"/multi": `<!doctype html><title>Multi</title>
<div role="listbox" id="lb" tabindex="0" aria-label="Fruit" aria-multiselectable="true" aria-activedescendant="o1">
<div role="option" id="o1" aria-selected="true">Apple</div>
<div role="option" id="o2" aria-selected="false">Banana</div>
<div role="option" id="o3">Cherry</div>
</div>
<script>
const lb = document.getElementById('lb');
const opts = Array.from(lb.children);
lb.addEventListener('keydown', (e) => {
  const i = opts.findIndex((o) => o.id === lb.getAttribute('aria-activedescendant'));
  if (e.key === 'ArrowDown' && i < opts.length - 1) lb.setAttribute('aria-activedescendant', opts[i + 1].id);
  else if (e.key === 'ArrowUp' && i > 0) lb.setAttribute('aria-activedescendant', opts[i - 1].id);
  else if (e.key === ' ') { const o = opts[i]; o.setAttribute('aria-selected', o.getAttribute('aria-selected') === 'true' ? 'false' : 'true'); }
  else return;
  e.preventDefault();
});
</script>`,
		"/doc": `<!doctype html><title>Doc</title>
<main>
<h1>Welcome</h1>
<p>Read the <a href="#terms">terms</a> first.</p>
<ul><li>Apples</li><li>Pears</li></ul>
<h2>Search</h2>
<label for="q">Query</label> <input id="q">
<button id="go" onclick="document.getElementById('out').textContent = 'Searched ' + document.getElementById('q').value">Go</button>
<p id="out"></p>
</main>
<script>window.keys = []; document.addEventListener('keydown', (e) => window.keys.push(e.key), true);</script>`,
		"/frame": `<!doctype html><title>Frame</title><button id="go">Go</button>
<iframe srcdoc="<div role=status id=s></div>"></iframe>`,
		"/alert": `<!doctype html><title>Alert</title><button id="go" onclick="
const a = document.createElement('div'); a.setAttribute('role', 'alert'); a.textContent = 'Error: name is required';
document.body.append(a)">Submit</button>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// liveEntry is a queue entry as the page-side rules produce it.
type liveEntry struct {
	Kind       string `json:"kind"`
	Politeness string `json:"politeness,omitempty"`
	Text       string `json:"text,omitempty"`
	nvdaOnly   bool
}

// TestE2ELiveRegionRules checks the page-side rules on their own: the page
// script is installed without StartAnnouncing, so nothing drains its queue,
// and each case's queue entries are read straight from it. want is the
// VoiceOver feed (the entries not flagged NVDA-only); the NVDA feed (every
// entry) is the same, except for the cases in nvdaDiffers.
func TestE2ELiveRegionRules(t *testing.T) {
	e2e(t)
	srv := liveServer(t)
	b, _ := open(t, engine.Chromium)
	if err := Navigate(b, srv.URL+"/blank"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.active.Evaluate(announcerJS); err != nil {
		t.Fatal(err)
	}
	live := func(p, text string) liveEntry { return liveEntry{Kind: "live", Politeness: p, Text: text} }
	cases := []struct {
		name, setup, change string
		want                []liveEntry
	}{
		{"polite inherited", `<div aria-live="polite"><p id="x"></p></div>`,
			`x.textContent = 'Saved'`, []liveEntry{live("polite", "Saved")}},
		{"assertive", `<div aria-live="assertive" id="x"></div>`,
			`const p = document.createElement('p'); p.textContent = 'Error: name is required'; x.append(p)`,
			[]liveEntry{live("assertive", "Error: name is required")}},
		{"alert", `<div role="alert" id="x"></div>`, `x.textContent = 'Oops'`, []liveEntry{live("assertive", "Oops")}},
		{"status", `<div role="status" id="x"></div>`, `x.textContent = '3 results available'`,
			[]liveEntry{live("polite", "3 results available")}},
		{"log", `<ul role="log" id="x"><li>one</li></ul>`,
			`const li = document.createElement('li'); li.textContent = 'two'; x.append(li)`, []liveEntry{live("polite", "two")}},
		{"explicit aria-live beats the role", `<div role="status" aria-live="assertive" id="x"></div>`,
			`x.textContent = 'Now'`, []liveEntry{live("assertive", "Now")}},
		{"off", `<div aria-live="off" id="x"></div>`, `x.textContent = 'quiet'`, nil},
		{"off beats the role", `<div role="status" aria-live="off" id="x"></div>`, `x.textContent = 'quiet'`, nil},
		{"not a region", `<div id="x"></div>`, `x.textContent = 'quiet'`, nil},
		{"timer is off", `<div role="timer" id="x"></div>`, `x.textContent = '0:01'`, nil},

		{"atomic speaks the whole region", `<div role="status">Page <span id="x">2</span> of 5</div>`,
			`x.textContent = '3'`, []liveEntry{live("polite", "Page 3 of 5")}},
		{"atomic text node", `<div aria-live="polite" aria-atomic="true" id="x">Page 2 of 5</div>`,
			`x.firstChild.data = 'Page 3 of 5'`, []liveEntry{live("polite", "Page 3 of 5")}},
		{"non-atomic speaks the change", `<div aria-live="polite">Page <span id="x">2</span> of 5</div>`,
			`x.textContent = '3'`, []liveEntry{live("polite", "3")}},
		{"aria-atomic=false on a status", `<div role="status" aria-atomic="false">Page <span id="x">2</span> of 5</div>`,
			`x.textContent = '3'`, []liveEntry{live("polite", "3")}},
		{"removals not relevant by default", `<div aria-live="polite" id="x"><p>gone</p></div>`,
			`x.firstChild.remove()`, nil},
		{"removals when relevant", `<div aria-live="polite" aria-relevant="removals" id="x"><p>gone</p></div>`,
			`x.firstChild.remove()`, []liveEntry{live("polite", "gone")}},
		{"additions only skips text changes", `<div aria-live="polite" aria-relevant="additions" id="x">a</div>`,
			`x.firstChild.data = 'b'`, nil},
		{"text only skips additions", `<div aria-live="polite" aria-relevant="text" id="x"></div>`,
			`x.append(document.createElement('p')); x.lastChild.textContent = 'new'`, nil},
		{"all", `<div aria-live="polite" aria-relevant="all" id="x"><p>old</p></div>`,
			`x.firstChild.remove()`, []liveEntry{live("polite", "old")}},

		{"aria-hidden", `<div aria-live="polite"><div aria-hidden="true" id="x"></div></div>`, `x.textContent = 'hidden'`, nil},
		{"display none", `<div aria-live="polite"><div style="display:none" id="x"></div></div>`, `x.textContent = 'hidden'`, nil},
		{"hidden attribute", `<div aria-live="polite"><div hidden id="x"></div></div>`, `x.textContent = 'hidden'`, nil},
		{"hidden region", `<div role="status" hidden id="x"></div>`, `x.textContent = 'hidden'`, nil},
		{"hidden parts left out", `<div aria-live="polite" id="x"></div>`,
			`x.innerHTML = '<p>seen <span aria-hidden="true">unseen</span><span style="display:none">gone</span></p>'`,
			[]liveEntry{live("polite", "seen")}},

		{"busy is silent", `<div role="status" aria-busy="true" id="x"></div>`, `x.textContent = 'loading'`, nil},
		{"busy ancestor is silent", `<section aria-busy="true"><div role="status" id="x"></div></section>`,
			`x.textContent = 'loading'`, nil},
		{"busy ends: spoken once", `<div aria-live="polite" aria-busy="true" id="x"></div>`,
			`x.textContent = 'step 1'; await tick(); x.append(' and 2'); await tick(); x.setAttribute('aria-busy', 'false')`,
			[]liveEntry{live("polite", "step 1 and 2")}},
		{"busy ends with a change", `<div role="status" aria-busy="true" id="x"></div>`,
			`x.textContent = 'a'; await tick(); x.textContent = 'done'; x.removeAttribute('aria-busy')`,
			[]liveEntry{live("polite", "done")}},

		{"status inserted filled is silent", `<div id="x"></div>`,
			`x.innerHTML = '<div role="status">Already here</div>'`, nil},
		{"alert inserted filled is spoken", `<div id="x"></div>`,
			`x.innerHTML = '<div role="alert">Error: required</div>'`, []liveEntry{live("assertive", "Error: required")}},
		{"alert inside an inserted wrapper", `<div id="x"></div>`,
			`x.innerHTML = '<section><p role="alert">Failed</p></section>'`, []liveEntry{live("assertive", "Failed")}},
		{"hidden alert inserted", `<div id="x"></div>`, `x.innerHTML = '<div role="alert" hidden>no</div>'`, nil},
		{"alert revealed", `<div role="alert" hidden id="x">Shown</div>`, `x.hidden = false`,
			[]liveEntry{live("assertive", "Shown")}},
		{"content revealed in a region", `<div aria-live="polite"><p hidden id="x">Revealed</p></div>`, `x.hidden = false`,
			[]liveEntry{live("polite", "Revealed")}},
		{"role becomes alert", `<div id="x">Now an alert</div>`, `x.setAttribute('role', 'alert')`,
			[]liveEntry{live("assertive", "Now an alert")}},

		{"blank", `<div aria-live="polite" id="x"></div>`, `x.textContent = '   '`, nil},
		{"identical text node replaced", `<div role="status" id="x">Saved</div>`, `x.textContent = 'Saved'`, nil},
		{"log message remounted", `<div role="log" aria-relevant="additions" id="x"><p>Hello</p></div>`,
			`x.firstChild.remove(); const p = document.createElement('p'); p.textContent = 'Hello'; x.append(p)`, nil},
		{"log message remounted in 3 tasks", `<div role="log" aria-relevant="additions" id="x"><p>Hello</p></div>`,
			`for (let i = 0; i < 3; i++) { x.innerHTML = '<p>Hello</p>'; await tick(); }`, nil},
		{"two messages remounted in one commit", `<div role="log" aria-relevant="additions" id="x"><p>Q</p><p>A</p></div>`,
			`x.innerHTML = '<p>Q</p><p>A</p>'`, nil},
		{"remounted inside an atomic element", `<div aria-live="polite"><div aria-atomic="true" id="x">Page <span>2</span> of 5</div></div>`,
			`x.querySelector('span').replaceWith(Object.assign(document.createElement('span'), {textContent: '2'}))`, nil},
		{"remount and a real change", `<div role="log" aria-relevant="additions" id="x"><p>Q</p></div>`,
			`x.innerHTML = '<p>Q</p>'; await tick(); x.insertAdjacentHTML('beforeend', '<p>A</p>')`,
			[]liveEntry{live("polite", "A")}},
		{"identical data", `<div aria-live="polite" id="x">Saved</div>`, `x.firstChild.data = 'Saved'`, nil},
		{"NBSP forces a repeat", `<div role="status" id="x">Saved</div>`, `x.textContent = 'Saved\u00a0'`,
			[]liveEntry{live("polite", "Saved")}},
		{"thin space forces a repeat", `<div aria-live="polite" id="x"><span>Copied</span></div>`,
			`x.replaceChildren(document.createElement('span')); x.firstChild.textContent = 'Copied\u2009'`,
			[]liveEntry{live("polite", "Copied")}},
		{"NBSP alone is blank", `<div aria-live="polite" id="x"></div>`, `x.textContent = '\u00a0'`, nil},
		{"alert in a fallback role list", `<div id="x"></div>`,
			`x.innerHTML = '<div><p role="alert status">Failed</p><p role="status alert">Not an alert</p></div>'`,
			[]liveEntry{live("assertive", "Failed")}},
		{"identical re-render with CSS-hidden text", `<style>.sr-skip{display:none}</style><div role="status" id="x"><p>Saved<span class="sr-skip"> (hidden)</span></p></div>`,
			`x.innerHTML = '<p>Saved<span class="sr-skip"> (hidden)</span></p>'`, nil},
		{"CSS-hidden addition is silent", `<style>.sr-skip{display:none}</style><div aria-live="polite" id="x"></div>`,
			`x.innerHTML = '<p class="sr-skip">hidden</p>'`, nil},
		{"CSS-hidden addition to an atomic region", `<style>.sr-skip{display:none}</style><div role="status" id="x">Saved</div>`,
			`x.insertAdjacentHTML('beforeend', '<span class="sr-skip">x</span>')`, nil},
		{"CSS-hidden removal from an atomic region", `<style>.sr-skip{display:none}</style><div role="status" id="x">Saved<span class="sr-skip">x</span></div>`,
			`x.lastChild.remove()`, nil},
		{"attribute-hidden removal when removals are relevant", `<div aria-live="polite" aria-relevant="all" id="x">Saved<span hidden>x</span></div>`,
			`x.lastChild.remove()`, nil},
		// A known limit: a removed node is detached, so its CSS can't be read,
		// and stylesheet-hidden text removed from a region that lists
		// removals is spoken. Erring towards an extra announcement beats
		// remembering rendered text that goes stale and silences a real one.
		{"known limit: CSS-hidden removal when removals are relevant",
			`<style>.sr-skip{display:none}</style><div aria-live="polite" aria-relevant="all" id="x">Saved<span class="sr-skip">x</span></div>`,
			`x.lastChild.remove()`, []liveEntry{live("polite", "x")}},
		{"removal from an atomic region isn't relevant by default", `<div role="status" id="x">Page 3 <span>of 5</span></div>`,
			`x.lastChild.remove()`, nil},
		{"removal from an atomic region when relevant", `<div role="status" aria-relevant="additions removals" id="x">Page 3 <span>of 5</span></div>`,
			`x.lastChild.remove()`, []liveEntry{live("polite", "Page 3")}},
		{"clear then set repeats", `<div role="status" id="x">Saved</div>`,
			`x.textContent = ''; await tick(); x.textContent = 'Saved'`, []liveEntry{live("polite", "Saved")}},
		{"coalesced re-render", `<div aria-live="polite" id="x"><span>1</span> <span>result</span></div>`,
			`x.replaceChildren(); for (const w of ['3', 'results', 'available']) { const s = document.createElement('span'); s.textContent = w; x.append(s, ' ') }`,
			[]liveEntry{live("polite", "3 results available")}},
		{"one entry per region", `<div role="status" id="x"></div><div role="alert" id="y"></div>`,
			`x.textContent = 'a'; y.textContent = 'b'; x.textContent = 'c'`,
			[]liveEntry{live("polite", "c"), live("assertive", "b")}},
		{"focus and live in order", `<input id="x"><div role="status" id="y"></div>`,
			`x.focus(); y.textContent = 'hint'`, []liveEntry{{Kind: "focus"}, live("polite", "hint")}},
		{"capped", `<div aria-live="polite" id="x"></div>`, `x.textContent = 'a'.repeat(5000)`,
			[]liveEntry{live("polite", strings.Repeat("a", 1024)+"…")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := b.active.Evaluate(`async ([setup, change]) => {
  const tick = () => new Promise((r) => setTimeout(r, 0));
  document.activeElement && document.activeElement.blur && document.activeElement.blur();
  document.body.innerHTML = setup;
  await tick(); await tick();
  const q = window.__aclioAnnouncer.q;
  q.length = 0;
  await (new Function('tick', 'return (async () => {' + change + '})()'))(tick);
  await tick(); await tick();
  return q.map((e) => e.kind === 'live' ? e : { kind: e.kind });
}`, []string{c.setup, withIDs(c.setup, c.change)})
			if err != nil {
				t.Fatal(err)
			}
			all := toEntries(got)
			var vo, nvda []liveEntry
			for _, e := range all {
				if !e.nvdaOnly {
					vo = append(vo, e)
				}
				e.nvdaOnly = false
				nvda = append(nvda, e)
			}
			if !reflect.DeepEqual(vo, c.want) {
				t.Errorf("VoiceOver entries = %+v, want %+v", vo, c.want)
			}
			want, ok := nvdaDiffers[c.name]
			if !ok {
				want = c.want
			}
			if !reflect.DeepEqual(nvda, want) {
				t.Errorf("NVDA entries = %+v, want %+v", nvda, want)
			}
		})
	}
}

// nvdaDiffers is the NVDA feed for the rules cases where it differs from
// VoiceOver's: nodes replaced by nodes with identical text, which NVDA reads.
var nvdaDiffers = map[string][]liveEntry{
	"identical text node replaced":             {liveAs("polite", "Saved")},
	"identical re-render with CSS-hidden text": {liveAs("polite", "Saved")},
	"log message remounted":                    {liveAs("polite", "Hello")},
	"log message remounted in 3 tasks":         {liveAs("polite", "Hello"), liveAs("polite", "Hello"), liveAs("polite", "Hello")},
	"two messages remounted in one commit":     {liveAs("polite", "Q A")},
	"remounted inside an atomic element":       {liveAs("polite", "Page 2 of 5")},
	"remount and a real change":                {liveAs("polite", "Q"), liveAs("polite", "A")},
}

func liveAs(politeness, text string) liveEntry {
	return liveEntry{Kind: "live", Politeness: politeness, Text: text}
}

// withIDs binds the setup's x and y ids as variables for change, since
// Function bodies don't see the window's named elements reliably.
func withIDs(setup, change string) string {
	var pre string
	for _, id := range []string{"x", "y"} {
		if strings.Contains(setup, `id="`+id+`"`) {
			pre += fmt.Sprintf("const %s = document.getElementById('%s'); ", id, id)
		}
	}
	return pre + change
}

func toEntries(v any) []liveEntry {
	list, _ := v.([]any)
	var out []liveEntry
	for _, x := range list {
		m, _ := x.(map[string]any)
		e := liveEntry{}
		e.Kind, _ = m["kind"].(string)
		e.Politeness, _ = m["politeness"].(string)
		e.Text, _ = m["text"].(string)
		e.nvdaOnly, _ = m["nvdaOnly"].(bool)
		out = append(out, e)
	}
	return out
}

// openAnnouncing opens Chromium on path, announcing, with what the start
// said taken.
func openAnnouncing(t *testing.T, srv *httptest.Server, path string) *Browser {
	t.Helper()
	b, _ := open(t, engine.Chromium)
	if err := Navigate(b, srv.URL+path); err != nil {
		t.Fatal(err)
	}
	if err := StartAnnouncing(b, ""); err != nil {
		t.Fatal(err)
	}
	TakeAnnouncements(b)
	return b
}

// do runs actions on b and fails the test if one fails.
func do(t *testing.T, b *Browser, actions ...action.Action) {
	t.Helper()
	for _, d := range Perform(b, actions) {
		if !d.Succeeded {
			t.Fatalf("%v failed: %s", d.Action.Type, *d.Reason)
		}
	}
}

// heard collects what VoiceOver says for a while: until it has said last, or
// for wait when last is "" (to show something stays silent).
func heard(t *testing.T, b *Browser, last string, wait time.Duration) []string {
	t.Helper()
	vo, _ := heardBoth(t, b, last, wait)
	return vo
}

// heardBoth collects what each screen reader says, until either has said
// last, or for wait when last is "".
func heardBoth(t *testing.T, b *Browser, last string, wait time.Duration) (vo, nvda []string) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		said := TakeAnnouncements(b)
		vo, nvda = append(vo, preset.VoiceOverSaid(said)...), append(nvda, preset.NVDASaid(said)...)
		ends := func(said []string) bool { return len(said) > 0 && said[len(said)-1] == last }
		if last != "" && (ends(vo) || ends(nvda)) {
			return vo, nvda
		}
		time.Sleep(50 * time.Millisecond)
	}
	return vo, nvda
}

func js(code string) action.Action {
	return action.Action{Type: action.TypeJavascript, Data: action.Javascript{Code: code}}
}

func press(key string) action.Action {
	return action.Action{Type: action.TypePress, Data: action.Press{Key: key}}
}

func click(sel string) action.Action {
	return action.Action{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: &sel}}}
}

func typeText(sel, text string) action.Action {
	return action.Action{Type: action.TypeType, Data: action.Typing{Target: &action.Target{Selector: &sel}, Text: text}}
}

// A status region updated from a debounced search is heard after the input's
// own announcements, and a search with no results is too.
func TestE2ELiveRegionSearch(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/search")
	do(t, b, click("#r"))
	if said := preset.VoiceOverSaid(TakeAnnouncements(b)); len(said) == 0 || said[0] != "Recipients" {
		t.Fatalf("focus said %q", said)
	}
	do(t, b, typeText("#r", "a"))
	said := heard(t, b, "2 results available", 3*time.Second)
	if len(said) < 2 || said[0] != "a" || said[len(said)-1] != "2 results available" || strings.Count(strings.Join(said, "|"), "results") != 1 {
		t.Errorf("said %q, want the value, then one 2 results available", said)
	}
	do(t, b, typeText("#r", "z"))
	if said := heard(t, b, "No matching users or groups.", 3*time.Second); said[len(said)-1] != "No matching users or groups." {
		t.Errorf("said %q", said)
	}
}

// react-select's log speaks its added spans as one announcement.
func TestE2ELiveRegionReactSelect(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/select")
	do(t, b, click("#s"))
	TakeAnnouncements(b)
	do(t, b, typeText("#s", "r"))
	want := "1 result available. Use Up and Down to choose options."
	if said := heard(t, b, want, 3*time.Second); len(said) == 0 || said[len(said)-1] != want {
		t.Errorf("said %q, want last %q", said, want)
	}
}

// An alert the page inserts filled is heard; so are focus moves and live
// announcements in one batch, in the order they happened.
func TestE2ELiveRegionAlertAndOrder(t *testing.T) {
	e2e(t)
	srv := liveServer(t)
	b := openAnnouncing(t, srv, "/alert")
	do(t, b, click("#go"))
	if said := heard(t, b, "Error: name is required", 3*time.Second); said[len(said)-1] != "Error: name is required" {
		t.Errorf("alert said %q", said)
	}

	b = openAnnouncing(t, srv, "/form")
	do(t, b,
		click("#a"),
		js(`(() => { const s = document.createElement('div'); s.setAttribute('role', 'status'); document.body.append(s); setTimeout(() => s.textContent = 'Draft saved', 0); })()`),
	)
	heard(t, b, "Draft saved", 3*time.Second)
	do(t, b,
		js(`document.querySelector('[role=status]').textContent = 'Saving'`),
		press("Tab"),
		js(`document.querySelector('[role=status]').textContent = 'Saved'`),
	)
	said := heard(t, b, "Saved", 3*time.Second)
	want := []string{"Saving", "Second", "edit text", "Saved"}
	if !reflect.DeepEqual(said, want) {
		t.Errorf("said %q, want %q", said, want)
	}
}

// Live regions speak in the active tab only: not in a background tab, nor
// in an iframe (not supported yet); after a switch, the new tab's do.
func TestE2ELiveRegionTabsAndFrames(t *testing.T) {
	e2e(t)
	srv := liveServer(t)
	b := openAnnouncing(t, srv, "/blank")
	status := `(() => { let s = document.getElementById('s'); if (!s) { s = document.createElement('div'); s.id = 's'; s.setAttribute('role', 'status'); document.body.append(s); } setTimeout(() => s.textContent = %q, 50); })()`
	do(t, b, js(fmt.Sprintf(status, "")))
	do(t, b,
		action.Action{Type: action.TypeTabOpen, Data: action.TabOpen{}},
		action.Action{Type: action.TypeGoTo, Data: action.GoTo{URL: srv.URL + "/blank"}},
		js(fmt.Sprintf(status, "")),
	)
	TakeAnnouncements(b)
	// The background tab (the first) updates: silent.
	if _, err := b.context.Pages()[0].Evaluate(fmt.Sprintf(status, "background")); err != nil {
		t.Fatal(err)
	}
	do(t, b, js(fmt.Sprintf(status, "foreground")))
	if said := heard(t, b, "", time.Second); !reflect.DeepEqual(said, []string{"foreground"}) {
		t.Errorf("said %q, want only foreground", said)
	}
	do(t, b, action.Action{Type: action.TypeTabSwitch, Data: action.TabSwitch{Index: 1}})
	TakeAnnouncements(b)
	do(t, b, js(fmt.Sprintf(status, "back")))
	if said := heard(t, b, "back", 3*time.Second); len(said) == 0 || said[len(said)-1] != "back" {
		t.Errorf("after the switch said %q", said)
	}

	b = openAnnouncing(t, srv, "/frame")
	do(t, b, js(`document.querySelector('iframe').contentDocument.getElementById('s').textContent = 'in a frame'`))
	if said := heard(t, b, "", time.Second); len(said) != 0 {
		t.Errorf("iframe said %q", said)
	}
}

// Focus announcements are unchanged and carry no stray live entries, and a
// browser that isn't announcing installs nothing.
func TestE2EAnnouncerFocusRegression(t *testing.T) {
	e2e(t)
	srv := liveServer(t)
	b := openAnnouncing(t, srv, "/form")
	do(t, b, press("Tab"), press("Tab"), typeText("#b", "x"))
	said, nvda := heardBoth(t, b, "", 500*time.Millisecond)
	want := []string{"main", "Prefs", "form", "First", "edit text", "Second", "edit text", "x"}
	if !reflect.DeepEqual(said, want) {
		t.Errorf("said %q, want %q", said, want)
	}
	// The same steps in NVDA's wording and order.
	if want := []string{"main landmark", "Prefs", "form", "First", "edit", "Second", "edit", "x"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("NVDA said %q, want %q", nvda, want)
	}

	quiet, _ := open(t, engine.Chromium)
	if err := Navigate(quiet, srv.URL+"/search"); err != nil {
		t.Fatal(err)
	}
	if got, err := quiet.active.Evaluate(`() => typeof window.__aclioAnnouncer`); err != nil || got != "undefined" {
		t.Errorf("not announcing: __aclioAnnouncer is %v, %v", got, err)
	}
	if _, err := Snapshot(quiet, preset.Perception{Tabs: true}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// NVDA hears a live region's messages remounted with identical text, every
// time; VoiceOver doesn't. Focus moves and genuinely new messages are the
// same in both, in the same order.
func TestE2ELiveRegionNVDA(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/chat")
	do(t, b,
		js(`rerender()`),
		press("Tab"),
		js(`rerender()`),
		js(`document.getElementById('log').insertAdjacentHTML('beforeend', '<p>New</p>')`),
		js(`rerender()`),
	)
	vo, nvda := heardBoth(t, b, "Q A New", 3*time.Second)
	if want := []string{"Ask", "button", "New"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("VoiceOver said %q, want %q", vo, want)
	}
	if want := []string{"Q A", "Ask", "button", "Q A", "New", "Q A New"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("NVDA said %q, want %q", nvda, want)
	}
}

// In a real multi-select listbox, NVDA says "not selected" for an unselected
// option, on arrival and when Space deselects it; VoiceOver doesn't. NVDA
// doesn't say "selected" on arriving at a selected option (VoiceOver does),
// and words the rest its own way ("list" for the listbox). An option with no
// aria-selected at all is reported as Chromium exposes it.
func TestE2ENotSelected(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/multi")
	do(t, b, press("Tab"))
	vo, nvda := heardBoth(t, b, "", time.Second)
	if want := []string{"Fruit", "list box", "Apple", "selected", "1 of 3"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("enter: VoiceOver %q, want %q", vo, want)
	}
	if want := []string{"Fruit", "list", "Apple", "1 of 3"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("enter: NVDA %q, want %q", nvda, want)
	}
	do(t, b, press("ArrowDown"))
	vo, nvda = heardBoth(t, b, "", time.Second)
	if want := []string{"Banana", "2 of 3"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("arrive on Banana: VoiceOver %q, want %q", vo, want)
	}
	if want := []string{"Banana", "not selected", "2 of 3"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("arrive on Banana: NVDA %q, want %q", nvda, want)
	}
	do(t, b, press("ArrowUp"), press("Space"))
	vo, nvda = heardBoth(t, b, "", time.Second)
	if want := []string{"Apple", "selected", "1 of 3"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("deselect Apple: VoiceOver %q, want %q", vo, want)
	}
	if want := []string{"Apple", "1 of 3", "not selected"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("deselect Apple: NVDA %q, want %q", nvda, want)
	}
	do(t, b, press("ArrowDown"), press("ArrowDown"))
	vo, nvda = heardBoth(t, b, "", time.Second)
	if want := []string{"Banana", "2 of 3", "Cherry", "3 of 3"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("on to Cherry: VoiceOver %q, want %q", vo, want)
	}
	if want := []string{"Banana", "not selected", "2 of 3", "Cherry", "not selected", "3 of 3"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("on to Cherry (no aria-selected): NVDA %q, want %q", nvda, want)
	}
}

// Switching NVDA to any mode but Focus turns VoiceOver off; NVDA goes on
// speaking. Back in Focus, VoiceOver speaks again.
func TestE2ENVDAModeTurnsVoiceOverOff(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/form")
	mode := func(m action.NVDAMode) action.Action {
		return action.Action{Type: action.TypeNVDAChangeMode, Data: action.NVDAChangeMode{Mode: m}}
	}
	// Speech: a mode whose own behaviour isn't simulated (Browse and Native
	// selection switch to focus mode by itself in a text field, see
	// TestE2EBrowseModeSwitching; Review says "Document review").
	do(t, b, press("Tab"), mode(action.NVDAModeSpeech), press("Tab"))
	vo, nvda := heardBoth(t, b, "", 500*time.Millisecond)
	if want := []string{"main", "Prefs", "form", "First", "edit text"}; !reflect.DeepEqual(vo, want) {
		t.Errorf("VoiceOver said %q, want only what it said before Speech %q", vo, want)
	}
	if want := []string{"main landmark", "Prefs", "form", "First", "edit", "Second", "edit"}; !reflect.DeepEqual(nvda, want) {
		t.Errorf("NVDA said %q, want %q", nvda, want)
	}
	if got := NVDAMode(b); got != action.NVDAModeSpeech {
		t.Errorf("mode = %q", got)
	}
	do(t, b, mode(action.NVDAModeFocus), press("Tab"))
	vo, nvda = heardBoth(t, b, "", 500*time.Millisecond)
	if want := []string{"Save", "button"}; !reflect.DeepEqual(vo, want) || !reflect.DeepEqual(nvda, want) {
		t.Errorf("back in Focus: VoiceOver %q, NVDA %q, want both %q", vo, nvda, want)
	}
}
