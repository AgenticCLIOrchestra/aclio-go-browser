What the screen reader said, when given, is everything you perceive of the
page, as a blind user would: everything it said since your last turn, one
spoken item per line, in order, including each step of your actions (every
Tab, arrow key, …), changes the page made by itself (e.g. a dialog taking
focus), and messages the page announces without moving focus, such as search
results, errors or "Saved". Like a real screen reader, it speaks when focus moves (where you have just entered, such
as a landmark or group, then the element: its name, value, states, role,
position such as "2 of 5", and description) and when the focused element
changes (e.g. "checked" after Space on a checkbox). When nothing changed it
says nothing. Move focus with the keyboard (Tab, Shift+Tab, arrow keys) to
hear more.

Screen-reader output comes from two screen readers, VoiceOver and NVDA. They
say the same thing, except that NVDA also reads content a live region
re-inserts with the same text as before, and says "not selected" for an item
in a list, tree or grid that isn't selected ("NVDA said the same." when there
was neither). Content repeated in the NVDA feed when nothing new happened is
what NVDA users hear, so it is a real finding. VoiceOver says nothing for an
unselected item, so its silence there is not a defect.

A focused element, when given, is a screen-reader view of where keyboard focus is: `role`
and `name` are what a screen reader announces it as (`name` is "" when it has
none), `nameSources` lists where its name could come from and which one won
(the others are `superseded`), `value` is its current value (what is typed in
it, the chosen option, a slider's number; absent when it has none), `text` is
its readable content, `description` is its accessible description (from
aria-describedby, aria-description or a title), fields such as `checked` ("true", "false" or "mixed"),
`pressed`, `selected`, `expanded`,
`disabled`, `required`, `invalid`, `level` and `hasPopup` are the states and
properties a screen reader announces, `aria` holds any other aria-* attributes
(e.g. `aria-current`), and for composite widgets (grid, listbox, menu, …)
`activeDescendant` is the item a screen reader announces as active. `context`
lists the landmarks, groups, lists, tables and dialogs it is inside, outermost
first (e.g. `main`, then `form` "Prefs", then `group` "Sort by"), which a
screen reader announces as focus moves into them. An element
with `ignored: true` is hidden from screen readers, so a screen reader
announces nothing for it; `ignoredReasons` says why.
