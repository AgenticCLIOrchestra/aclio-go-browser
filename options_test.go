package browser

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestResolveDirsGiven(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "nested", "work")
	artifacts := filepath.Join(root, "out")

	d, err := ResolveDirs(Options{WorkingDir: &working, ArtifactsDir: &artifacts})
	if err != nil {
		t.Fatal(err)
	}
	if d.Working != working || d.Artifacts != artifacts {
		t.Errorf("dirs = %+v", d)
	}
	if info, err := os.Stat(working); err != nil || !info.IsDir() {
		t.Errorf("working dir not created: %v", err)
	}
	if _, err := os.Stat(artifacts); !os.IsNotExist(err) {
		t.Errorf("artifacts dir should be created lazily, stat err = %v", err)
	}
}

func TestResolveDirsRelativeBecomesAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	d, err := ResolveDirs(Options{WorkingDir: ptr("work"), ArtifactsDir: ptr("art")})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(d.Working) || !filepath.IsAbs(d.Artifacts) {
		t.Errorf("dirs not absolute: %+v", d)
	}
}

func TestResolveDirsDefaults(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	d, err := ResolveDirs(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(d.Working) != filepath.Clean(os.TempDir()) {
		t.Errorf("working dir %q not under temp dir %q", d.Working, os.TempDir())
	}
	if !strings.HasPrefix(filepath.Base(d.Working), tempPrefix) {
		t.Errorf("working dir %q lacks prefix", d.Working)
	}
	if d.Artifacts != filepath.Join(d.Working, artifactsSubdir) {
		t.Errorf("artifacts = %q", d.Artifacts)
	}
}

// Temp working dirs must sort alphabetically in creation order, including
// across a change in the number of digits anywhere in the timestamp.
func TestTempWorkingDirsSortChronologically(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	base := time.Date(2026, 1, 9, 9, 59, 59, 999_999_999, time.UTC)
	times := []time.Time{
		base,
		base.Add(time.Nanosecond),      // rolls into 10:00:00.000000000
		base.Add(time.Hour * 24 * 365), // next year
		base.Add(time.Hour*24*365 + time.Nanosecond),
	}
	var names []string
	for _, ts := range times {
		dir, err := newTempWorkingDir(ts)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.Base(dir))
	}
	if !slices.IsSorted(names) {
		t.Errorf("names not in chronological order: %v", names)
	}
	for _, n := range names {
		if strings.ContainsAny(n, `:/\`) {
			t.Errorf("name %q has a character some OS rejects", n)
		}
	}
}

func TestResolveDirsSameInstantStaysUnique(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	now := time.Now()
	a, err := newTempWorkingDir(now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newTempWorkingDir(now)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two runs at the same instant got the same dir")
	}
}

func TestResolveDirsEmptyPointerIsError(t *testing.T) {
	work := t.TempDir()
	cases := map[string]Options{
		"WorkingDir":   {WorkingDir: ptr("")},
		"ArtifactsDir": {WorkingDir: &work, ArtifactsDir: ptr("")},
		"UserDataDir":  {WorkingDir: &work, UserDataDir: ptr("")},
	}
	for name, o := range cases {
		if _, err := ResolveDirs(o); err == nil {
			t.Errorf("empty %s should be an error", name)
		}
	}
}

// A bad option must fail before a temp working dir is created for nothing.
func TestResolveDirsFailsBeforeCreatingTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if _, err := ResolveDirs(Options{UserDataDir: ptr("")}); err == nil {
		t.Fatal("want error")
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("temp dir created despite the error: %v", entries)
	}
}

func TestResolveDirsUserData(t *testing.T) {
	work := t.TempDir()

	d, err := ResolveDirs(Options{WorkingDir: &work})
	if err != nil {
		t.Fatal(err)
	}
	if d.UserData != nil {
		t.Errorf("nil UserDataDir should mean an anonymous session, got %q", *d.UserData)
	}
	if entries, _ := os.ReadDir(work); len(entries) != 0 {
		t.Errorf("anonymous session left files in the working dir: %v", entries)
	}

	t.Chdir(t.TempDir())
	d, err = ResolveDirs(Options{WorkingDir: &work, UserDataDir: ptr("profile")})
	if err != nil {
		t.Fatal(err)
	}
	if d.UserData == nil || !filepath.IsAbs(*d.UserData) {
		t.Fatalf("UserData = %v, want an absolute path", d.UserData)
	}
	if _, err := os.Stat(*d.UserData); !os.IsNotExist(err) {
		t.Errorf("Chrome data dir should be left for Chrome to create, stat err = %v", err)
	}
}
