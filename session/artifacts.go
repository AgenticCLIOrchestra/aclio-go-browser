package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"unicode/utf8"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// Artifact files are reached only through an os.Root on the artifacts
// directory, which refuses any path, symlink or ".." that would resolve
// outside it. Names are also checked with action.ValidateFileName first, so a
// bad name fails with a clear message rather than a filesystem error.

// maxArtifactRead caps how much of an artifact readArtifact returns to the
// agent, so one large file can't flood its context.
const maxArtifactRead = 1 << 20

// createArtifact writes data to a new artifact name. It never overwrites: an
// existing artifact of the same name is an error.
func createArtifact(dir, name string, data []byte) error {
	if err := action.ValidateFileName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating artifacts dir: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("opening artifacts dir: %w", err)
	}
	defer root.Close()

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("artifact %q already exists; artifacts can't be overwritten, pick another name", name)
	}
	if err != nil {
		return fmt.Errorf("creating artifact %q: %w", name, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("writing artifact %q: %w", name, err)
	}
	return f.Close()
}

// readArtifact returns the content of artifact name. Binary artifacts (e.g.
// screenshots) can't be returned as text and are refused.
func readArtifact(dir, name string) (string, error) {
	if err := action.ValidateFileName(name); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("artifact %q does not exist", name)
	}
	if err != nil {
		return "", fmt.Errorf("opening artifacts dir: %w", err)
	}
	defer root.Close()

	info, err := root.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("artifact %q does not exist", name)
	}
	if err != nil {
		return "", fmt.Errorf("reading artifact %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact %q is not a regular file", name)
	}
	if info.Size() > maxArtifactRead {
		return "", fmt.Errorf("artifact %q is %d bytes, more than the %d that can be read", name, info.Size(), maxArtifactRead)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("reading artifact %q: %w", name, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("artifact %q is binary and can't be read as text", name)
	}
	return string(data), nil
}

// listArtifacts returns the sorted names of every artifact (regular files
// only), as a JSON array. A directory that doesn't exist yet means none.
func listArtifacts(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return "", fmt.Errorf("listing artifacts: %w", err)
	}
	names := []string{}
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	b, err := json.Marshal(names)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
