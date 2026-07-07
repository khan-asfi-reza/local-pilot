package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"harness/harness/agent"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

// commands are the slash commands shown as hints and completed with Tab.
var commands = []string{"/plan", "/ask", "/auto", "/model", "/cwd", "/clear", "/help", "/quit"}

// spinnerFrames are the braille glyphs cycled while the model or a tool works.
var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

const (
	inputMinRows = 1
	inputMaxRows = 10
)

// confirmResult carries the user's answer to an ask-mode confirmation back to
// the waiting agent goroutine.
type confirmResult struct {
	decision tools.Decision
	feedback string
}

// ui holds the terminal application and all its live state.
type ui struct {
	app      *tview.Application
	root     *tview.Flex
	top      *tview.Pages
	out      *tview.TextView
	notice   *tview.TextView
	activity *tview.TextView
	hint     *tview.TextView
	status   *tview.TextView
	input    *tview.TextArea
	ag       *agent.Agent
	session  *Session
	workDir  string
	allowed  []string

	// noticeTimer clears the ephemeral command-output panel after a while.
	noticeTimer *time.Timer

	// bottom is what sits below the status line: the input box normally, or a
	// confirmation selector / feedback box while approving. bottomKind names it.
	bottom     tview.Primitive
	bottomKind string
	// skipNextDiff suppresses re-rendering a diff on the tool result when it was
	// already shown for approval.
	skipNextDiff bool

	busy   bool
	cancel context.CancelFunc

	// follow keeps the transcript pinned to the bottom as new output arrives.
	// Scrolling up pauses it; a new submit or paging down resumes it.
	follow bool

	// turn timing, token count, and outcome, for the live loader and completion.
	turnStart time.Time
	turnErr   bool
	canceled  bool
	tokens    int

	doing       string
	spinnerStop chan struct{}

	// confirm delivers the confirmation answer. Buffered so the UI goroutine
	// never blocks when it resolves a choice.
	confirm chan confirmResult
}

// newUI builds the terminal application, styled dark to blend with the terminal
// and shaped like a chat client: a scrolling transcript above a bordered,
// expanding input box.
func newUI(ag *agent.Agent, session *Session, workDir string) *ui {
	// Keep the terminal's own background; give borders rounded corners and a muted
	// palette so nothing reads as a loud solid block.
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorGray
	tview.Styles.MoreContrastBackgroundColor = tcell.ColorGray
	tview.Styles.BorderColor = tcell.ColorGray
	tview.Styles.TitleColor = tcell.ColorGray
	tview.Styles.PrimaryTextColor = tcell.ColorDefault
	tview.Styles.SecondaryTextColor = tcell.ColorGray
	tview.Borders.TopLeft = '╭'
	tview.Borders.TopRight = '╮'
	tview.Borders.BottomLeft = '╰'
	tview.Borders.BottomRight = '╯'

	u := &ui{
		app:     tview.NewApplication(),
		ag:      ag,
		session: session,
		workDir: workDir,
		allowed: ag.ToolNames(),
		confirm: make(chan confirmResult, 1),
		follow:  true,
	}

	u.out = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)
	u.out.SetBackgroundColor(tcell.ColorDefault)
	u.out.SetBorderPadding(0, 0, 1, 1)

	// The notice panel shows ephemeral command output (help, cwd, model list) and
	// clears on the next input or after a minute, so it never clutters the chat.
	u.notice = tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	u.notice.SetBackgroundColor(tcell.ColorDefault)
	u.notice.SetBorder(true)

	// The transcript and the notice panel share the content area; only one shows.
	u.top = tview.NewPages().
		AddPage("transcript", u.out, true, true).
		AddPage("notice", u.notice, true, false)

	u.activity = tview.NewTextView().SetDynamicColors(true)
	u.activity.SetBackgroundColor(tcell.ColorDefault)

	// A slash-command hint row, shown only while a command is being typed. It has
	// its own line so it works even while the model is thinking (the spinner owns
	// the activity line).
	u.hint = tview.NewTextView().SetDynamicColors(true)
	u.hint.SetBackgroundColor(tcell.ColorDefault)

	u.status = tview.NewTextView().SetDynamicColors(true)
	u.status.SetBackgroundColor(tcell.ColorDefault)

	u.input = tview.NewTextArea()
	u.input.SetPlaceholder("Type a task, or / for commands")
	u.input.SetBackgroundColor(tcell.ColorDefault)
	u.input.SetBorder(true)
	u.input.SetBorderColor(tcell.ColorGray)
	u.input.SetChangedFunc(u.onInputChanged)
	u.input.SetInputCapture(u.inputKeys)
	u.bottom = u.input

	u.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(u.top, 0, 1, false).
		AddItem(u.activity, 1, 0, false).
		AddItem(u.status, 1, 0, false).
		AddItem(u.hint, 0, 0, false).
		AddItem(u.input, 3, 0, true)

	u.app.SetInputCapture(u.keys)
	// Bracketed paste delivers a clipboard as one event, so a paste inserts in a
	// single step instead of arriving as slow character-by-character key presses.
	u.app.EnablePaste(true)
	// Mouse lets the transcript scroll with the wheel.
	u.app.EnableMouse(true)
	u.app.SetRoot(u.root, true).SetFocus(u.input)

	u.setStatus()
	u.banner()
	return u
}

// run starts the event loop.
func (u *ui) run() error { return u.app.Run() }

// keys is the global key handler. Ctrl-C cancels a running turn or closes an
// overlay, and quits only when idle. Tab cycles the mode on an empty prompt.
func (u *ui) keys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyCtrlC:
		if u.busy || u.bottomKind != "input" {
			u.cancelTurn()
		} else {
			u.app.Stop()
		}
		return nil
	case tcell.KeyBacktab:
		// Shift-Tab cycles the mode when the input is focused; during a
		// confirmation it navigates the selector.
		if u.bottomKind != "input" {
			return ev
		}
		u.cycleMode()
		return nil
	case tcell.KeyPgUp:
		// Page keys always scroll the transcript (the change under review sits
		// there during a confirmation).
		u.follow = false
		scrollBy(u.out, -10)
		return nil
	case tcell.KeyPgDn:
		u.follow = true
		scrollBy(u.out, 10)
		return nil
	}
	return ev
}

// inputKeys handles keys while the input box is focused: Enter submits, Alt-Enter
// adds a newline, and Tab either completes a slash command or cycles the mode.
func (u *ui) inputKeys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEnter:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			u.input.SetText(u.input.GetText()+"\n", true)
			return nil
		}
		text := u.input.GetText()
		u.input.SetText("", false)
		u.onInputChanged()
		u.handle(text)
		return nil
	case tcell.KeyTab:
		u.tryComplete()
		return nil
	}
	return ev
}

// tryComplete completes a partial slash command to its first match.
func (u *ui) tryComplete() bool {
	t := strings.TrimSpace(u.input.GetText())
	if !strings.HasPrefix(t, "/") || strings.Contains(t, " ") {
		return false
	}
	for _, c := range commands {
		if strings.HasPrefix(c, t) {
			u.input.SetText(c+" ", true)
			u.onInputChanged()
			return true
		}
	}
	return false
}

// onInputChanged grows the input box to fit its content and shows slash-command
// hints while a command is being typed.
func (u *ui) onInputChanged() {
	_, _, w, _ := u.input.GetInnerRect()
	rows := countRows(u.input.GetText(), w)
	u.root.ResizeItem(u.input, clamp(rows, inputMinRows, inputMaxRows)+2, 0)

	// Slash-command hints, on their own row so they appear even while the model
	// is thinking (the spinner owns the activity row).
	t := strings.TrimSpace(u.input.GetText())
	if strings.HasPrefix(t, "/") && !strings.Contains(t, " ") {
		var matches []string
		for _, c := range commands {
			if strings.HasPrefix(c, t) {
				matches = append(matches, c)
			}
		}
		u.hint.SetText(" [aqua::b]" + strings.Join(matches, "   ") + "[-:-:-]")
		u.root.ResizeItem(u.hint, 1, 0)
	} else {
		u.hint.SetText("")
		u.root.ResizeItem(u.hint, 0, 0)
	}
}

// showNotice displays ephemeral command output in the notice panel, which clears
// on the next input or after a minute so it never clutters the transcript.
func (u *ui) showNotice(title string, render func(w io.Writer)) {
	u.notice.SetTitle(" " + title + "  (clears automatically) ")
	u.notice.Clear()
	render(u.notice)
	u.notice.ScrollToBeginning()
	u.top.SwitchToPage("notice")
	if u.noticeTimer != nil {
		u.noticeTimer.Stop()
	}
	u.noticeTimer = time.AfterFunc(60*time.Second, func() {
		u.app.QueueUpdateDraw(u.clearNotice)
	})
}

// clearNotice hides the notice panel and shows the transcript again.
func (u *ui) clearNotice() {
	if u.noticeTimer != nil {
		u.noticeTimer.Stop()
		u.noticeTimer = nil
	}
	u.top.SwitchToPage("transcript")
}

// handle routes a submitted line to a command or a new turn.
func (u *ui) handle(text string) {
	// Any input clears an open notice panel first.
	if u.noticeTimer != nil {
		u.clearNotice()
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return
	}
	// Any submission resumes following the transcript to the bottom.
	u.follow = true
	if strings.HasPrefix(t, "/") {
		u.command(t)
		return
	}
	if u.busy {
		u.writeln("[yellow]Busy; wait for the current turn to finish[-]")
		return
	}
	u.startTurn(t)
}

// startTurn appends the user message, echoes it, and runs the agent in a
// goroutine with a cancelable context so Ctrl-C can stop it mid-flight.
func (u *ui) startTurn(text string) {
	u.busy = true
	u.turnStart = time.Now()
	u.turnErr = false
	u.canceled = false
	u.tokens = 0
	u.session.Messages = append(u.session.Messages, model.Message{Role: "user", Content: text})
	fmt.Fprintf(u.out, "\n[silver]────────────────────────────────────[-]\n")
	fmt.Fprintf(u.out, "[aqua::b]❯ %s[-:-:-]\n\n", tview.Escape(text))
	u.out.ScrollToEnd()
	u.startSpinner("Thinking…")

	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	go func() {
		req := agent.Request{
			Messages: u.session.Messages,
			Allowed:  u.allowed,
			Mode:     u.session.Mode,
			WorkDir:  u.workDir,
		}
		updated := u.ag.Run(ctx, req, u.emit, u.confirmFn)
		u.app.QueueUpdateDraw(func() {
			u.stopSpinner()
			u.session.Messages = updated
			_ = u.session.save(u.workDir)
			u.printOutcome()
			if u.follow {
				u.out.ScrollToEnd()
			}
			u.busy = false
			u.cancel = nil
		})
	}()
}

// printOutcome shows a completion line with an icon and a human-readable
// duration, reflecting how the turn ended.
func (u *ui) printOutcome() {
	elapsed := humanDuration(time.Since(u.turnStart))
	suffix := ""
	if u.tokens > 0 {
		suffix = fmt.Sprintf(" · %d tokens", u.tokens)
	}
	switch {
	case u.canceled:
		fmt.Fprintf(u.out, "\n[gray]⊘ Stopped after %s%s[-]\n", elapsed, suffix)
	case u.turnErr:
		fmt.Fprintf(u.out, "\n[red]✗ Failed after %s%s[-]\n", elapsed, suffix)
	default:
		fmt.Fprintf(u.out, "\n[green]✓ Completed in %s%s[-]\n", elapsed, suffix)
	}
}

// humanDuration renders a duration in a short, readable form.
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		s := int(d.Seconds() + 0.5)
		if s < 1 {
			s = 1
		}
		return fmt.Sprintf("%ds", s)
	case d < time.Hour:
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// startSpinner shows an animated indicator with a label describing what the
// agent is doing. It runs on a ticker until stopSpinner is called.
func (u *ui) startSpinner(label string) {
	u.doing = label
	stop := make(chan struct{})
	u.spinnerStop = stop
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-stop:
				u.app.QueueUpdateDraw(func() { u.activity.SetText("") })
				return
			case <-ticker.C:
				f := spinnerFrames[frame%len(spinnerFrames)]
				frame++
				u.app.QueueUpdateDraw(func() {
					meta := humanDuration(time.Since(u.turnStart))
					if u.tokens > 0 {
						meta += fmt.Sprintf(" · %d tokens", u.tokens)
					}
					u.activity.SetText(fmt.Sprintf(" [aqua]%c[-] [white]%s[-] [gray](%s · Ctrl-C to stop)[-]", f, tview.Escape(u.doing), meta))
				})
			}
		}
	}()
}

// stopSpinner ends the spinner goroutine. Safe to call more than once.
func (u *ui) stopSpinner() {
	if u.spinnerStop != nil {
		close(u.spinnerStop)
		u.spinnerStop = nil
	}
}

// cancelTurn aborts the in-flight turn: it cancels the context, closes any
// overlay, and unblocks a pending confirmation.
func (u *ui) cancelTurn() {
	u.stopSpinner()
	u.canceled = true
	if u.cancel != nil {
		u.cancel()
	}
	if u.bottomKind != "input" {
		u.restoreInput()
	}
	select {
	case u.confirm <- confirmResult{tools.Decline, ""}:
	default:
	}
	u.out.ScrollToEnd()
}

// emit renders one harness event on the UI goroutine.
func (u *ui) emit(ev events.Event) {
	u.app.QueueUpdateDraw(func() {
		switch ev.Type {
		case "text":
			fmt.Fprint(u.out, tview.Escape(ev.Content))
		case "tool_call":
			u.skipNextDiff = false
			label := u.toolLabel(ev.Tool, ev.Info)
			u.doing = label
			if ev.Info != "" {
				u.doing = label + " " + ev.Info
			}
			fmt.Fprintf(u.out, "\n[green]●[-] [white::b]%s[-:-:-]([silver]%s[-])\n", label, tview.Escape(ev.Info))
		case "tool_result":
			u.doing = "Thinking…"
			u.renderResult(ev)
		case "usage":
			u.tokens = ev.Tokens
		case "error":
			if !strings.Contains(ev.Message, "context canceled") {
				u.turnErr = true
				fmt.Fprintf(u.out, "\n[red::b]error:[-:-:-] [red]%s[-]\n", tview.Escape(ev.Message))
			}
		}
		if u.follow {
			u.out.ScrollToEnd()
		}
	})
}

// confirmFn shows a mutating action for approval and blocks until the user
// chooses. In auto mode it approves immediately. The change is shown full-width
// in the transcript; the input area becomes a selector. The loader reads
// "Waiting for approval".
func (u *ui) confirmFn(tool, summary string, diff *events.Diff) (tools.Decision, string) {
	if u.session.Mode == "auto" {
		return tools.Approve, ""
	}
	u.app.QueueUpdateDraw(func() {
		u.doing = "Waiting for approval"
		fmt.Fprintf(u.out, "\n[yellow::b]Review and approve:[-:-:-] [silver]%s[-]\n", tview.Escape(summary))
		if diff != nil {
			u.renderDiffTo(u.out, diff)
			u.skipNextDiff = true // do not render it again on the result
		}
		u.out.ScrollToEnd()
		u.showConfirm()
	})
	r := <-u.confirm
	return r.decision, r.feedback
}

// showConfirm docks a clear selector where the input was: Accept, Reject, or ask
// the agent to do something else. Arrow keys move; Enter chooses.
func (u *ui) showConfirm() {
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetMainTextColor(tcell.ColorDefault).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorLightSkyBlue)
	list.SetBorder(true).SetTitle(" Approve this change?  (↑↓ choose · Tab scrolls the change) ")
	list.AddItem("Accept", "", 'a', func() { u.resolveConfirm(tools.Approve, "") })
	list.AddItem("Reject", "", 'r', func() { u.resolveConfirm(tools.Decline, "") })
	list.AddItem("Ask agent what to do instead", "", 't', func() { u.showFeedback() })
	// Tab moves focus to the transcript so up/down scroll the change there; Tab
	// again returns to the selector. Up/down only act on the focused pane.
	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyTab || ev.Key() == tcell.KeyBacktab {
			u.app.SetFocus(u.out)
			return nil
		}
		return ev
	})
	u.out.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyTab || ev.Key() == tcell.KeyBacktab {
			u.app.SetFocus(list)
			return nil
		}
		return ev
	})
	u.swapBottom(list, 5, "confirm")
}

// showFeedback swaps the selector for a text box; what the user types is sent
// back to the agent as guidance, and the action is declined.
func (u *ui) showFeedback() {
	field := tview.NewTextArea()
	field.SetPlaceholder("Tell the agent what to do instead (Enter to send, Esc to cancel)")
	field.SetBackgroundColor(tcell.ColorDefault)
	field.SetBorder(true).SetTitle(" Ask agent ")
	field.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyEnter:
			if ev.Modifiers()&tcell.ModAlt != 0 {
				field.SetText(field.GetText()+"\n", true)
				return nil
			}
			u.resolveConfirm(tools.Decline, field.GetText())
			return nil
		case tcell.KeyEsc:
			u.resolveConfirm(tools.Decline, "")
			return nil
		}
		return ev
	})
	u.swapBottom(field, 5, "feedback")
}

// resolveConfirm restores the input and delivers the decision to the agent.
func (u *ui) resolveConfirm(d tools.Decision, feedback string) {
	u.restoreInput()
	u.confirm <- confirmResult{d, feedback}
}

// swapBottom replaces the bottom item (the input) with another primitive.
func (u *ui) swapBottom(p tview.Primitive, height int, kind string) {
	u.root.RemoveItem(u.bottom)
	u.bottom = p
	u.bottomKind = kind
	u.root.AddItem(p, height, 0, true)
	u.app.SetFocus(p)
}

// restoreInput docks the input box back and focuses it.
func (u *ui) restoreInput() {
	u.out.SetInputCapture(nil) // drop the confirm-time Tab handler
	u.root.RemoveItem(u.bottom)
	u.bottom = u.input
	u.bottomKind = "input"
	_, _, w, _ := u.input.GetInnerRect()
	rows := clamp(countRows(u.input.GetText(), w), inputMinRows, inputMaxRows) + 2
	u.root.AddItem(u.input, rows, 0, true)
	u.app.SetFocus(u.input)
}

// scrollBy moves a text view's scroll position by delta rows.
func scrollBy(tv *tview.TextView, delta int) {
	row, col := tv.GetScrollOffset()
	row += delta
	if row < 0 {
		row = 0
	}
	tv.ScrollTo(row, col)
}

// toolLabel maps a tool name to a human-friendly verb, matching the familiar
// coding-agent style (Read, Write, Update, Run, Search).
func (u *ui) toolLabel(tool, info string) string {
	switch tool {
	case "read_file":
		return "Read"
	case "edit_file":
		return "Update"
	case "write_file":
		// A write over an existing file reads as an update; a new file as a write.
		if info != "" && fileExists(filepath.Join(u.workDir, info)) {
			return "Update"
		}
		return "Write"
	case "search":
		return "Search"
	case "list_dir":
		return "List"
	case "shell_run", "code_run":
		return "Run"
	case "web_search":
		return "Search web"
	case "load_skill":
		return "Skill"
	default:
		return tool
	}
}

// renderResult prints the indented result line under a tool call (the ⎿ line),
// then any diff or command output.
func (u *ui) renderResult(ev events.Event) {
	detail, color := resultDetail(ev)
	fmt.Fprintf(u.out, "  [gray]⎿[-] [%s]%s[-]\n", color, tview.Escape(detail))
	if ev.Diff != nil {
		if u.skipNextDiff {
			u.skipNextDiff = false // already shown for approval
		} else {
			u.renderDiffTo(u.out, ev.Diff)
		}
		return
	}
	u.renderExecOutput(ev.Data)
}

// resultDetail summarizes a tool result in one human, capitalized line. Changes
// are reported in lines, not bytes.
func resultDetail(ev events.Event) (string, string) {
	if d := ev.Diff; d != nil {
		switch {
		case d.Removed == 0:
			return fmt.Sprintf("Added %d lines", d.Added), "silver"
		case d.Added == 0:
			return fmt.Sprintf("Removed %d lines", d.Removed), "silver"
		default:
			return fmt.Sprintf("Updated, +%d -%d lines", d.Added, d.Removed), "silver"
		}
	}
	var m map[string]any
	if json.Unmarshal([]byte(ev.Data), &m) == nil {
		if e, ok := m["error"].(string); ok {
			// Show only the first sentence; the full guidance is for the model.
			return capitalize(firstSentence(e)), "red"
		}
		if ec, ok := m["exit_code"].(float64); ok {
			if int(ec) == 0 {
				return "Ran successfully", "lime"
			}
			return fmt.Sprintf("Failed (exit %d)", int(ec)), "red"
		}
		if c, ok := m["content"].(string); ok {
			return fmt.Sprintf("Read %d lines", countLines(c)), "silver"
		}
		if matches, ok := m["matches"].([]any); ok {
			return fmt.Sprintf("Found %d matches", len(matches)), "silver"
		}
		if entries, ok := m["entries"].([]any); ok {
			return fmt.Sprintf("Listed %d entries", len(entries)), "silver"
		}
		if _, ok := m["body"]; ok {
			return "Loaded skill", "silver"
		}
	}
	if ev.Info != "" {
		return capitalize(ev.Info), "silver"
	}
	return "Done", "silver"
}

// capitalize upper-cases the first character of a string.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// countLines counts the lines in text, ignoring a trailing newline.
func countLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// renderExecOutput prints stdout and stderr for an executing tool, if present.
// The exit code is already shown in the result line.
func (u *ui) renderExecOutput(data string) {
	var m map[string]any
	if json.Unmarshal([]byte(data), &m) != nil {
		return
	}
	if _, ok := m["exit_code"]; !ok {
		return
	}
	ec, _ := m["exit_code"].(float64)
	if int(ec) == 0 {
		return // success: the status line is enough, no output spam
	}
	// On failure show just the last few lines (usually the actual error), not the
	// whole log. The model still gets the full output to debug.
	out, _ := m["stderr"].(string)
	if strings.TrimSpace(out) == "" {
		out, _ = m["stdout"].(string)
	}
	u.writeTail(out, 6)
}

// writeTail prints the last n non-empty lines of text, dimmed, with a note if
// earlier lines were dropped.
func (u *ui) writeTail(text string, n int) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		fmt.Fprintf(u.out, "[gray]    … %d earlier lines[-]\n", len(lines)-n)
		lines = lines[len(lines)-n:]
	}
	for _, line := range lines {
		fmt.Fprintf(u.out, "    [red]%s[-]\n", tview.Escape(line))
	}
}

// fileExists reports whether a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// firstSentence returns a short version of a message for the UI: the first line
// or sentence, hard-capped, so a long model-directed error never floods.
func firstSentence(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	const max = 60
	if len(s) > max {
		s = strings.TrimSpace(s[:max]) + "…"
	}
	return s
}

// maxDiffLines caps how many diff lines are shown in the transcript, so a large
// change stays readable when scrolling back.
const maxDiffLines = 15

// renderDiffTo draws a git-style diff to w: a green plus for additions, a red
// minus for removals, and a line-number gutter. Added and context code is
// syntax-highlighted; removed code stays red. It shows at most maxDiffLines and
// notes how many were hidden.
func (u *ui) renderDiffTo(w io.Writer, d *events.Diff) {
	fmt.Fprintf(w, "[yellow::b]%s[-:-:-] [gray](+%d -%d)[-]\n", tview.Escape(d.Path), d.Added, d.Removed)
	total := 0
	for _, h := range d.Hunks {
		total += len(h.Lines)
	}
	shown := 0
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			if shown >= maxDiffLines {
				fmt.Fprintf(w, "[gray]  … %d more lines[-]\n", total-shown)
				return
			}
			switch l.Op {
			case events.OpAdd:
				fmt.Fprintf(w, "[green]%4d +[-] %s\n", l.New, highlight(l.Text))
			case events.OpRemove:
				fmt.Fprintf(w, "[red]%4d - %s[-]\n", l.Old, tview.Escape(l.Text))
			default:
				fmt.Fprintf(w, "[gray]%4d  [-] %s\n", l.Old, highlight(l.Text))
			}
			shown++
		}
	}
}

// codeKeywords is a language-spanning set of common keywords, colored to make
// generated code readable rather than a wall of one color.
var codeKeywords = buildKeywordSet(`
def class return if else elif for while do switch case break continue pass with try except finally raise throw catch lambda yield
func type struct interface package import from as go defer chan map range var const let new typeof instanceof export default extends implements
function fn pub impl trait enum mut use mod match loop where async await move dyn public private protected static void
true false none null nil self this super and or not in is`)

func buildKeywordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// highlight applies basic per-line syntax coloring and returns a tview-tagged,
// escaped string: comments muted, strings olive, numbers aqua, keywords fuchsia.
func highlight(line string) string {
	var b strings.Builder
	emit := func(color, s string) {
		if color == "" {
			b.WriteString(tview.Escape(s))
		} else {
			b.WriteString("[" + color + "]" + tview.Escape(s) + "[-]")
		}
	}
	r := []rune(line)
	i, n := 0, len(r)
	for i < n {
		c := r[i]
		switch {
		case c == '#' || (c == '/' && i+1 < n && r[i+1] == '/'):
			emit("gray", string(r[i:])) // line comment to end
			i = n
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < n {
				if r[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if r[j] == c {
					j++
					break
				}
				j++
			}
			emit("olive", string(r[i:j]))
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && (isDigit(r[j]) || r[j] == '.' || (r[j] >= 'a' && r[j] <= 'f') || (r[j] >= 'A' && r[j] <= 'F') || r[j] == 'x') {
				j++
			}
			emit("aqua", string(r[i:j]))
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(r[j]) {
				j++
			}
			word := string(r[i:j])
			if codeKeywords[word] {
				emit("fuchsia", word)
			} else {
				emit("", word)
			}
			i = j
		default:
			emit("", string(c))
			i++
		}
	}
	return b.String()
}

func isDigit(c rune) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c rune) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c rune) bool  { return isIdentStart(c) || isDigit(c) }

// command handles a slash command typed at the prompt.
func (u *ui) command(cmd string) {
	fields := strings.Fields(cmd)
	switch fields[0] {
	case "/help":
		u.help()
	case "/plan":
		u.setMode("plan")
	case "/ask":
		u.setMode("ask")
	case "/auto":
		u.setMode("auto")
	case "/clear":
		u.session.Messages = nil
		_ = u.session.save(u.workDir)
		u.out.Clear()
		u.banner()
	case "/model":
		u.modelCmd(fields[1:])
	case "/cwd":
		u.showNotice("Working directory", func(w io.Writer) {
			fmt.Fprintf(w, "[white]%s[-]\n", tview.Escape(u.workDir))
		})
	case "/quit", "/exit":
		u.app.Stop()
	default:
		u.showNotice("Unknown command", func(w io.Writer) {
			fmt.Fprintf(w, "[red]%s is not a command.[-] Try /help.\n", tview.Escape(fields[0]))
		})
	}
}

// modelCmd lists the registered models with live running status, or switches the
// active one. Switching to a model whose backend is not running warns how to
// start it.
func (u *ui) modelCmd(args []string) {
	if len(args) == 0 {
		u.showNotice("Models", func(w io.Writer) {
			for _, m := range u.ag.Models() {
				marker := "  "
				if m.Active {
					marker = "[green]➤ [-]"
				}
				state := "[gray]not installed[-]"
				if m.Running {
					state = "[lime]ready[-]"
				}
				fmt.Fprintf(w, "%s[white]%-22s[-] %s  [gray]%s[-]\n", marker, tview.Escape(m.Name), state, tview.Escape(m.URL))
			}
			fmt.Fprintf(w, "\n[silver]Switch with /model <name>. Add one: ./pilot add <base-model>[-]\n")
		})
		return
	}
	name := args[0]
	if err := u.ag.SetModel(name); err != nil {
		u.showNotice("Model", func(w io.Writer) { fmt.Fprintf(w, "[red]%s[-]\n", tview.Escape(err.Error())) })
		return
	}
	u.session.Model = name
	_ = u.session.save(u.workDir)
	u.setStatus()
	u.showNotice("Model", func(w io.Writer) {
		if u.ag.Reachable(name) {
			fmt.Fprintf(w, "[green]Now using %s[-]\n", tview.Escape(name))
		} else {
			fmt.Fprintf(w, "[yellow]Now using %s, but ollama is not running.[-]\nStart it: ./pilot start\n", tview.Escape(name))
		}
	})
}

// setMode changes the permission mode. Only the human does this; the model
// cannot escalate itself.
func (u *ui) setMode(mode string) {
	u.session.Mode = mode
	_ = u.session.save(u.workDir)
	u.setStatus()
}

// cycleMode steps plan to ask to auto and back.
func (u *ui) cycleMode() {
	next := map[string]string{"plan": "ask", "ask": "auto", "auto": "plan"}
	m := next[u.session.Mode]
	if m == "" {
		m = "ask"
	}
	u.setMode(m)
}

// setStatus refreshes the status line: the model on the left as a colored chip,
// then the mode chip. The working directory is not shown (use /cwd).
func (u *ui) setStatus() {
	mode := map[string]string{
		"plan": "[black:blue] Plan [-:-]",
		"ask":  "[black:yellow] Ask [-:-]",
		"auto": "[black:green] Auto [-:-]",
	}[u.session.Mode]
	if mode == "" {
		mode = "[black:yellow] Ask [-:-]"
	}
	model := "[black:teal] " + u.ag.ActiveModel() + " [-:-]"
	u.status.SetText(fmt.Sprintf(" %s  %s", model, mode))
}

// banner prints the startup header.
func (u *ui) banner() {
	u.writeln("[aqua::b]local-pilot[-:-:-] [silver]local coding assistant[-]")
	active := u.ag.ActiveModel()
	if u.ag.Reachable(active) {
		u.writeln("[silver]Model [white]%s[-] [lime]ready[-]", tview.Escape(active))
	} else {
		u.writeln("[yellow]ollama is not running. Start it: ./pilot start   (see /model)[-]")
	}
	u.writeln("[silver]Type a task, /help for commands, Shift-Tab to cycle mode, Ctrl-C to stop a turn.[-]")
}

// help lists the commands and keys.
func (u *ui) help() {
	u.showNotice("Help", func(w io.Writer) {
		fmt.Fprintln(w, "[aqua::b]Commands[-:-:-]")
		fmt.Fprintln(w, "  [white]/plan /ask /auto[-]   set the permission mode")
		fmt.Fprintln(w, "  [white]/model[-]             list models; /model <name> switches")
		fmt.Fprintln(w, "  [white]/cwd[-]               show the working directory")
		fmt.Fprintln(w, "  [white]/clear[-]             forget the conversation")
		fmt.Fprintln(w, "  [white]/quit[-]              exit")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "[aqua::b]Keys[-:-:-]")
		fmt.Fprintln(w, "  [white]Enter[-]  send      [white]Alt-Enter[-]  newline")
		fmt.Fprintln(w, "  [white]Shift-Tab[-]  cycle mode      [white]Tab[-]  complete a command")
		fmt.Fprintln(w, "  [white]PgUp/PgDn[-]  scroll      [white]Ctrl-C[-]  stop a turn / exit")
		fmt.Fprintln(w, "  On a confirmation: [white]↑↓[-] move, [white]Enter[-] choose")
	})
}

// writeln writes a colored line to the output and keeps it scrolled to the end.
func (u *ui) writeln(format string, a ...any) {
	fmt.Fprintf(u.out, format+"\n", a...)
	u.out.ScrollToEnd()
}

// countRows estimates how many display rows a text needs at a given width, so
// the input box can grow to fit.
func countRows(text string, width int) int {
	if width < 1 {
		width = 80
	}
	rows := 0
	for _, line := range strings.Split(text, "\n") {
		n := utf8.RuneCountInString(line)
		r := n / width
		if n%width != 0 || r == 0 {
			r++
		}
		rows += r
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
