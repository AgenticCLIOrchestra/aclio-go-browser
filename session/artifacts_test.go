package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateArtifactIsExclusive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "artifacts")

	if err := createArtifact(dir, "notes.md", []byte("first")); err != nil {
		t.Fatal(err)
	}
	err := createArtifact(dir, "notes.md", []byte("second"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second create: err = %v, want already exists", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "notes.md")); string(got) != "first" {
		t.Errorf("artifact was overwritten: %q", got)
	}
}

func TestCreateArtifactMakesDirLazily(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dir should not exist yet")
	}
	if err := createArtifact(dir, "x.txt", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); err != nil {
		t.Error(err)
	}
}

func TestArtifactNamesCantEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "artifacts")
	for _, name := range []string{"../escape.txt", "a/b.txt", `..\x`, "..", "."} {
		if err := createArtifact(dir, name, []byte("x")); err == nil {
			t.Errorf("create %q: want error", name)
		}
		if _, err := readArtifact(dir, name); err == nil {
			t.Errorf("read %q: want error", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Error("a file was written outside the artifacts dir")
	}
}

// A symlink planted inside the artifacts dir must not let a create or read
// reach outside it.
func TestArtifactSymlinksCantEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "artifacts")
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	if err := os.Symlink(filepath.Join(root, "new.txt"), filepath.Join(dir, "dangling.txt")); err != nil {
		t.Fatal(err)
	}

	if got, err := readArtifact(dir, "link.txt"); err == nil {
		t.Errorf("read through symlink succeeded: %q", got)
	}
	if err := createArtifact(dir, "dangling.txt", []byte("x")); err == nil {
		t.Error("create through a dangling symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Error("a file was created outside the artifacts dir")
	}
}

func TestReadArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := createArtifact(dir, "a.md", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if got, err := readArtifact(dir, "a.md"); err != nil || got != "hello" {
		t.Errorf("read = %q, %v", got, err)
	}
	if _, err := readArtifact(dir, "missing.md"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing: err = %v", err)
	}
	if _, err := readArtifact(filepath.Join(dir, "nope"), "a.md"); err == nil {
		t.Error("read from a missing dir should fail")
	}

	if err := createArtifact(dir, "shot.png", []byte{0x89, 'P', 'N', 'G', 0xff, 0xfe}); err != nil {
		t.Fatal(err)
	}
	if _, err := readArtifact(dir, "shot.png"); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Errorf("binary: err = %v", err)
	}

	if err := createArtifact(dir, "big.txt", []byte(strings.Repeat("x", maxArtifactRead+1))); err != nil {
		t.Fatal(err)
	}
	if _, err := readArtifact(dir, "big.txt"); err == nil {
		t.Error("oversized read should fail")
	}
}

func TestListArtifacts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "artifacts")
	if got, err := listArtifacts(dir); err != nil || got != "[]" {
		t.Errorf("missing dir: %q, %v", got, err)
	}
	for _, n := range []string{"b.txt", "a.png"} {
		if err := createArtifact(dir, n, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := listArtifacts(dir); err != nil || got != `["a.png","b.txt"]` {
		t.Errorf("list = %q, %v", got, err)
	}
}
