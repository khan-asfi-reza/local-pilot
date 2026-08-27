package systemtest

import (
	"strings"
	"testing"

	"harness/harness/events"
	"harness/harness/tools"
)

// approve is a confirmation callback that records what it was shown and says yes.
type approve struct {
	calls    int
	tool     string
	summary  string
	diff     *events.Diff
	decision tools.Decision
	feedback string
}

func (a *approve) fn(tool, summary string, diff *events.Diff) (tools.Decision, string) {
	a.calls++
	a.tool, a.summary, a.diff = tool, summary, diff
	return a.decision, a.feedback
}

// TestPlanModeIsReadOnly checks the plan-mode gate: every mutating tool is
// refused and the project on disk is untouched, while a read still works.
func TestPlanModeIsReadOnly(t *testing.T) {
	dir := tempProject(t, map[string]string{"main.py": "print('hi')\n"})
	reg := tools.NewRegistry(nil)
	e := env(dir)

	mutations := []struct {
		name string
		args map[string]any
	}{
		{"write_file", map[string]any{"path": "new.py", "content": "x = 1\n"}},
		{"edit_file", map[string]any{"path": "main.py", "edits": []map[string]string{{"old_text": "hi", "new_text": "bye"}}}},
		{"shell_run", map[string]any{"command": "touch created-by-shell"}},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			out, _ := reg.Dispatch(call(t, m.name, m.args), reg.Names(), tools.ModePlan, e, nil)
			if !strings.Contains(errorOf(t, out), "Plan mode is read-only") {
				t.Fatalf("%s was not refused in plan mode: %s", m.name, out)
			}
		})
	}
	if exists(dir, "new.py") || exists(dir, "created-by-shell") {
		t.Fatal("plan mode created a file")
	}
	if got := body(t, dir, "main.py"); got != "print('hi')\n" {
		t.Fatalf("plan mode changed main.py: %q", got)
	}

	out, _ := reg.Dispatch(call(t, "read_file", map[string]any{"path": "main.py"}), reg.Names(), tools.ModePlan, e, nil)
	if errorOf(t, out) != "" {
		t.Fatalf("read_file should work in plan mode: %s", out)
	}
}

// TestPlanModeHidesMutatingTools checks the model is never even offered a
// mutating tool in plan mode, so it cannot loop on refusals.
func TestPlanModeHidesMutatingTools(t *testing.T) {
	reg := tools.NewRegistry(nil)

	defs := reg.Defs(nil, false)
	for _, d := range defs {
		switch d.Function.Name {
		case "write_file", "edit_file", "shell_run", "serve", "npm_install", "install_deps":
			t.Fatalf("plan-mode tool definitions still offer %q", d.Function.Name)
		}
	}
	if len(defs) == 0 {
		t.Fatal("plan mode offered no tools at all")
	}
	if len(reg.Defs(nil, true)) <= len(defs) {
		t.Fatal("auto mode should offer strictly more tools than plan mode")
	}

	docs, names := reg.Describe(nil, false)
	for _, n := range names {
		if n == "write_file" {
			t.Fatal("plan-mode Describe still lists write_file")
		}
	}
	if !strings.Contains(docs, "read_file") {
		t.Fatal("plan-mode docs dropped read_file")
	}
}

// TestAskModePreviewsBeforeWriting checks that ask mode pauses on a mutation,
// shows a real diff first, and only writes after approval.
func TestAskModePreviewsBeforeWriting(t *testing.T) {
	dir := tempProject(t, map[string]string{"calc.py": "def add(a, b):\n    return a - b\n"})
	reg := tools.NewRegistry(nil)
	e := env(dir)
	// Read first: read-before-modify is a separate gate, satisfied here on purpose.
	reg.Dispatch(call(t, "read_file", map[string]any{"path": "calc.py"}), reg.Names(), tools.ModeAsk, e, nil)

	edit := call(t, "edit_file", map[string]any{
		"path":  "calc.py",
		"edits": []map[string]string{{"old_text": "return a - b", "new_text": "return a + b"}},
	})

	declined := &approve{decision: tools.Decline, feedback: "wrong file"}
	out, _ := reg.Dispatch(edit, reg.Names(), tools.ModeAsk, e, declined.fn)
	if declined.calls != 1 {
		t.Fatalf("ask mode did not ask (calls=%d)", declined.calls)
	}
	if declined.diff == nil || declined.diff.Added != 1 || declined.diff.Removed != 1 {
		t.Fatalf("confirmation carried no usable diff: %+v", declined.diff)
	}
	if !strings.Contains(declined.summary, "calc.py") {
		t.Fatalf("confirmation summary does not name the file: %q", declined.summary)
	}
	msg := errorOf(t, out)
	if !strings.Contains(msg, "declined") || !strings.Contains(msg, "wrong file") {
		t.Fatalf("decline did not reach the model with its feedback: %q", msg)
	}
	if strings.Contains(body(t, dir, "calc.py"), "a + b") {
		t.Fatal("a declined edit was still applied")
	}

	accepted := &approve{decision: tools.Approve}
	out, diff := reg.Dispatch(edit, reg.Names(), tools.ModeAsk, e, accepted.fn)
	if errorOf(t, out) != "" {
		t.Fatalf("approved edit failed: %s", out)
	}
	if diff == nil || diff.Path != "calc.py" {
		t.Fatalf("approved edit returned no diff to render: %+v", diff)
	}
	if !strings.Contains(body(t, dir, "calc.py"), "a + b") {
		t.Fatal("approved edit was not applied")
	}
}

// TestAutoModeNeverAsks checks auto mode runs a mutation straight through.
func TestAutoModeNeverAsks(t *testing.T) {
	dir := tempProject(t, nil)
	reg := tools.NewRegistry(nil)
	asked := &approve{decision: tools.Decline}

	out, _ := reg.Dispatch(
		call(t, "write_file", map[string]any{"path": "src/app.py", "content": "x = 1\n"}),
		reg.Names(), tools.ModeAuto, env(dir), asked.fn)

	if asked.calls != 0 {
		t.Fatalf("auto mode asked for confirmation %d times", asked.calls)
	}
	if errorOf(t, out) != "" {
		t.Fatalf("auto-mode write failed: %s", out)
	}
	if got := body(t, dir, "src/app.py"); got != "x = 1\n" {
		t.Fatalf("auto-mode write produced %q", got)
	}
}

// TestAllowedSetGateRunsFirst checks a tool outside the request's allowed set is
// refused before the mode gate is consulted, and the message names the real set.
func TestAllowedSetGateRunsFirst(t *testing.T) {
	dir := tempProject(t, nil)
	reg := tools.NewRegistry(nil)

	out, _ := reg.Dispatch(
		call(t, "shell_run", map[string]any{"command": "rm -rf ."}),
		[]string{"read_file", "search"}, tools.ModeAuto, env(dir), nil)

	msg := errorOf(t, out)
	if !strings.Contains(msg, "does not exist in this task") {
		t.Fatalf("expected an allowed-set refusal, got %q", msg)
	}
	if !strings.Contains(msg, "read_file") {
		t.Fatalf("refusal should list the tools that ARE available, got %q", msg)
	}
}

// TestReadBeforeModify checks the harness refuses to overwrite a file the model
// has not looked at this turn, and lifts the block once it has read it.
func TestReadBeforeModify(t *testing.T) {
	dir := tempProject(t, map[string]string{"config.py": "DEBUG = True\n"})
	reg := tools.NewRegistry(nil)
	e := env(dir)

	blind := call(t, "write_file", map[string]any{"path": "config.py", "content": "DEBUG = False\n"})
	out, _ := reg.Dispatch(blind, reg.Names(), tools.ModeAuto, e, nil)
	if !strings.Contains(errorOf(t, out), "Read the file first") {
		t.Fatalf("blind overwrite was allowed: %s", out)
	}
	if body(t, dir, "config.py") != "DEBUG = True\n" {
		t.Fatal("blind overwrite changed the file")
	}

	// A NEW file is exempt: there is nothing to read.
	out, _ = reg.Dispatch(call(t, "write_file", map[string]any{"path": "fresh.py", "content": "y = 2\n"}), reg.Names(), tools.ModeAuto, e, nil)
	if errorOf(t, out) != "" {
		t.Fatalf("writing a new file should not need a prior read: %s", out)
	}

	reg.Dispatch(call(t, "read_file", map[string]any{"path": "config.py"}), reg.Names(), tools.ModeAuto, e, nil)
	out, _ = reg.Dispatch(blind, reg.Names(), tools.ModeAuto, e, nil)
	if errorOf(t, out) != "" {
		t.Fatalf("write still refused after reading: %s", out)
	}
	if body(t, dir, "config.py") != "DEBUG = False\n" {
		t.Fatal("write after read was not applied")
	}
}

// TestOutsideWorkDirNeedsApproval checks a shell command touching a path outside
// the project pauses even in auto mode, and is refused outright when no one can
// be asked (the headless path).
func TestOutsideWorkDirNeedsApproval(t *testing.T) {
	dir := tempProject(t, nil)
	reg := tools.NewRegistry(nil)
	outside := call(t, "shell_run", map[string]any{"command": "cat ../../etc/hosts"})

	asked := &approve{decision: tools.Decline}
	out, _ := reg.Dispatch(outside, reg.Names(), tools.ModeAuto, env(dir), asked.fn)
	if asked.calls != 1 {
		t.Fatalf("an escaping command did not pause in auto mode (calls=%d)", asked.calls)
	}
	if !strings.Contains(asked.summary, "OUTSIDE the working directory") {
		t.Fatalf("confirmation did not flag the escape: %q", asked.summary)
	}
	if !strings.Contains(errorOf(t, out), "declined") {
		t.Fatalf("decline was not reported: %s", out)
	}

	out, _ = reg.Dispatch(outside, reg.Names(), tools.ModeAuto, env(dir), nil)
	if !strings.Contains(errorOf(t, out), "outside the working directory") {
		t.Fatalf("headless run should refuse an escaping command outright: %s", out)
	}
}

// TestLoadSkillOnlyWhenSkillsExist checks the skill tool is registered only when
// there is at least one skill, so its name enum is never empty.
func TestLoadSkillOnlyWhenSkillsExist(t *testing.T) {
	if tools.NewRegistry(nil).Get("load_skill") != nil {
		t.Fatal("load_skill registered with no skills available")
	}
	reg := tools.NewRegistry([]string{"react", "serving"})
	if reg.Get("load_skill") == nil {
		t.Fatal("load_skill missing although skills exist")
	}
	docs, _ := reg.Describe([]string{"load_skill"}, true)
	if !strings.Contains(docs, "react") || !strings.Contains(docs, "serving") {
		t.Fatalf("load_skill schema does not enumerate the skills: %s", docs)
	}
}
