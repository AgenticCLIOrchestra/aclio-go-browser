package action

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

// MaxWait is the longest Wait that validates.
const MaxWait = time.Minute

// maxFileName is the longest file name most filesystems accept, in bytes.
const maxFileName = 255

// Validate checks that a is performable: a known type whose Data is that
// type's payload struct, and a payload that satisfies its rules (a well-formed
// target where one is needed, a safe file name, a navigable URL, …). Every
// caller that performs actions should call it first, so bad actions are
// refused the same way everywhere. It does not check what an agent was
// allowed to do; see CheckTargeting and the preset package for that.
func Validate(a Action) error {
	e, ok := lookup(a.Type)
	if !ok {
		return fmt.Errorf("unknown action type %q", a.Type)
	}
	if a.Data == nil || reflect.TypeOf(a.Data) != reflect.TypeOf(e.payload) {
		return fmt.Errorf("%s: data is %T, want %T", a.Type, a.Data, e.payload)
	}
	if err := validateData(a.Data); err != nil {
		return fmt.Errorf("%s: %w", a.Type, err)
	}
	return nil
}

func validateData(d Data) error {
	switch d := d.(type) {
	case Click:
		return validateTarget(d.Target)
	case DoubleClick:
		return validateTarget(d.Target)
	case Hover:
		return validateTarget(d.Target)
	case MouseDown:
		return validateOptionalTarget(d.Target)
	case MouseUp:
		return validateOptionalTarget(d.Target)
	case Scroll:
		if d.DeltaX == 0 && d.DeltaY == 0 {
			return errors.New("deltaX or deltaY must be non-zero")
		}
	case Typing:
		if d.Text == "" {
			return errors.New("text is empty")
		}
		return validateOptionalTarget(d.Target)
	case Fill:
		return validateTarget(d.Target)
	case Press:
		if strings.TrimSpace(d.Key) == "" {
			return errors.New("key is empty")
		}
	case Wait:
		if d.Milliseconds <= 0 || time.Duration(d.Milliseconds)*time.Millisecond > MaxWait {
			return fmt.Errorf("milliseconds must be between 1 and %d", MaxWait.Milliseconds())
		}
	case GoTo:
		return ValidateURL(d.URL)
	case TabSwitch:
		return validateIndex(d.Index)
	case TabClose:
		return validateIndex(d.Index)
	case Javascript:
		if strings.TrimSpace(d.Code) == "" {
			return errors.New("code is empty")
		}
	case Screenshot:
		if d.FullPage && d.Target != nil {
			return errors.New("fullPage and target are mutually exclusive")
		}
		if err := validateOptionalTarget(d.Target); err != nil {
			return err
		}
		if err := ValidateFileName(d.Name); err != nil {
			return err
		}
		// The file is {name}.png, which must fit the name limit too.
		return ValidateFileName(ScreenshotFileName(d))
	case CreateArtifact:
		return ValidateFileName(d.Name)
	case ReadArtifact:
		return ValidateFileName(d.Name)
	}
	return nil
}

func validateTarget(t Target) error {
	switch {
	case t.Selector == nil && t.Point == nil:
		return errors.New("target needs a selector or a point")
	case t.Selector != nil && t.Point != nil:
		return errors.New("target takes a selector or a point, not both")
	case t.Selector != nil && strings.TrimSpace(*t.Selector) == "":
		return errors.New("target selector is empty")
	}
	return nil
}

func validateOptionalTarget(t *Target) error {
	if t == nil {
		return nil
	}
	return validateTarget(*t)
}

func validateIndex(i int) error {
	if i < 1 {
		return fmt.Errorf("index must be 1 or more, got %d", i)
	}
	return nil
}

// ValidateURL checks a goTo target. Relative references (path, root-relative,
// query or fragment) are allowed, as are absolute http and https URLs. Every
// other scheme is refused: javascript: would run script even when the
// javascript action is not allowed, and file:, data:, chrome: and the like
// reach beyond the web page the agent is meant to drive.
func ValidateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("url is empty")
	}
	// Browsers strip surrounding whitespace before parsing, so " javascript:…"
	// would slip past a scheme check done on the raw string.
	if raw != strings.TrimSpace(raw) {
		return fmt.Errorf("url %q has surrounding whitespace", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url %q: %w", raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "", "http", "https":
		return nil
	default:
		return fmt.Errorf("url %q: scheme %q is not allowed, only http and https", raw, u.Scheme)
	}
}

// ValidateFileName checks a name the agent gave for a file in the artifacts
// directory. It must be a plain file name: not empty, not "." or "..", no
// path separators (either kind), no drive or stream colon, no NUL, and at
// most 255 bytes. That keeps every artifact inside the artifacts directory
// whatever the agent sends, e.g. "../../.bashrc" is refused.
func ValidateFileName(name string) error {
	switch {
	case name == "":
		return errors.New("file name is empty")
	case name == "." || name == "..":
		return fmt.Errorf("file name %q is not a file", name)
	case strings.ContainsAny(name, `/\:`):
		return fmt.Errorf("file name %q must not contain /, \\ or :", name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("file name %q contains NUL", name)
	case len(name) > maxFileName:
		return fmt.Errorf("file name is %d bytes, at most %d allowed", len(name), maxFileName)
	}
	return nil
}

// ScreenshotFileName is the artifact file name a Screenshot is saved as:
// {Name}.png.
func ScreenshotFileName(s Screenshot) string {
	return s.Name + ".png"
}

// CheckTargeting reports an error if a uses a kind of Target that targeting
// does not allow. It assumes a is otherwise valid.
func CheckTargeting(a Action, targeting Targeting) error {
	t := targetOf(a.Data)
	if t == nil {
		return nil
	}
	if t.Selector != nil && !targeting.Selector {
		return fmt.Errorf("%s: targeting by selector is not allowed", a.Type)
	}
	if t.Point != nil && !targeting.Point {
		return fmt.Errorf("%s: targeting by point is not allowed", a.Type)
	}
	return nil
}

// targetOf returns d's target, or nil if it has none.
func targetOf(d Data) *Target {
	switch d := d.(type) {
	case Click:
		return &d.Target
	case DoubleClick:
		return &d.Target
	case Hover:
		return &d.Target
	case Fill:
		return &d.Target
	case MouseDown:
		return d.Target
	case MouseUp:
		return d.Target
	case Typing:
		return d.Target
	case Screenshot:
		return d.Target
	}
	return nil
}
