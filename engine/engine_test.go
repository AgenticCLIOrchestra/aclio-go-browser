package engine

import "testing"

func TestResolve(t *testing.T) {
	cases := map[Browser]Browser{
		"":       Chromium,
		Chromium: Chromium,
		Firefox:  Firefox,
	}
	for in, want := range cases {
		got, err := Resolve(in)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []Browser{"webkit", "Chromium", "chrome"} {
		if _, err := Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) should fail", bad)
		}
	}
}

func TestAllReturnsCopy(t *testing.T) {
	a := All()
	a[0] = "mutated"
	if All()[0] != Chromium {
		t.Fatal("All shares its slice between calls")
	}
}
