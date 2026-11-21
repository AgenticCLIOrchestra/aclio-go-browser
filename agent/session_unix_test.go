//go:build unix

package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aclio "github.com/agenticcliorchestra/aclio-go"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// fakeClaude puts a `claude` first on PATH that answers each stream-json user
// message with a result whose structured_output is taskComplete only from the
// second turn, and records its argv in args.txt.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" >> '` + dir + `/args.txt'
n=0
while IFS= read -r line; do
  case "$line" in *'"type":"user"'*)
    n=$((n+1))
    done=false; [ $n -ge 2 ] && done=true
    printf '{"type":"result","subtype":"success","session_id":"conv","total_cost_usd":0.0%d,"usage":{"input_tokens":1,"output_tokens":1},"structured_output":{"taskComplete":%s}}\n' "$n" "$done" ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// Both turns of a task run on one session: one CLI process, locked-down
// tools and the preset's schema, with no SessionID needed for the second.
func TestTurnsOnOneSession(t *testing.T) {
	cli := fakeClaude(t)
	req := initial(t, preset.Default())
	req.Request.LogWriter = io.Discard

	sess, err := StartSession(req.Request, req.Preset)
	if err != nil {
		t.Fatal(err)
	}
	defer aclio.CloseSession(sess)

	req.Session = sess
	turn, err := Initial(req)
	if err != nil || turn.Response.TaskComplete {
		t.Fatalf("initial: %+v, %v", turn, err)
	}
	turn, err = Subsequent(SubsequentRequest{
		Prompt:  DefaultSubsequentPrompt,
		Preset:  req.Preset,
		State:   req.State,
		Request: req.Request,
		Session: sess,
	})
	if err != nil || !turn.Response.TaskComplete || turn.SessionID != "conv" {
		t.Fatalf("subsequent: %+v, %v", turn, err)
	}

	args, _ := os.ReadFile(filepath.Join(cli, "args.txt"))
	a := string(args)
	if strings.Count(a, "--input-format\n") != 1 {
		t.Errorf("want one CLI process, args:\n%s", a)
	}
	if !strings.Contains(a, "--allowedTools\nRead(./**)\n") || !strings.Contains(a, `"taskComplete"`) {
		t.Errorf("tools or schema missing, args:\n%s", a)
	}
}

func TestStartSessionChecks(t *testing.T) {
	req := initial(t, preset.Default())
	r := req.Request
	r.Dir = "relative"
	if _, err := StartSession(r, req.Preset); err == nil {
		t.Error("relative dir should fail")
	}
	if _, err := StartSession(req.Request, preset.Preset{}); err == nil {
		t.Error("invalid preset should fail")
	}
}
