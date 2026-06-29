package tools

import "harness/harness/events"

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

type rawOp struct {
	kind byte // 'c' context, '+' add, '-' remove
	text string
}

// diffLines computes a line-level diff between two slices using a longest common
// subsequence, then walks it to produce an ordered sequence of context, add, and
// remove operations.
func diffLines(a, b []string) []rawOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []rawOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, rawOp{'c', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, rawOp{'-', a[i]})
			i++
		default:
			ops = append(ops, rawOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, rawOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, rawOp{'+', b[j]})
	}
	return ops
}

// buildDiff turns the old and new text of a file into a positional diff with
// real line numbers and hunks grouped around the changes.
func buildDiff(path, oldText, newText string) *events.Diff {
	ops := diffLines(splitLines(oldText), splitLines(newText))
	var lines []events.DiffLine
	oldNo, newNo := 1, 1
	added, removed := 0, 0
	for _, op := range ops {
		switch op.kind {
		case 'c':
			lines = append(lines, events.DiffLine{Op: events.OpContext, Old: oldNo, New: newNo, Text: op.text})
			oldNo++
			newNo++
		case '-':
			lines = append(lines, events.DiffLine{Op: events.OpRemove, Old: oldNo, Text: op.text})
			oldNo++
			removed++
		case '+':
			lines = append(lines, events.DiffLine{Op: events.OpAdd, New: newNo, Text: op.text})
			newNo++
			added++
		}
	}
	return &events.Diff{Path: path, Added: added, Removed: removed, Hunks: groupHunks(lines, 3)}
}

// groupHunks slices the flat line list into hunks, each covering a changed
// region plus up to ctx lines of surrounding context, merging regions that
// overlap.
func groupHunks(lines []events.DiffLine, ctx int) []events.Hunk {
	n := len(lines)
	var changed []int
	for i := 0; i < n; i++ {
		if lines[i].Op != events.OpContext {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	type span struct{ lo, hi int }
	cur := span{clampLo(changed[0] - ctx), clampHi(changed[0]+ctx, n)}
	var spans []span
	for _, c := range changed[1:] {
		lo, hi := clampLo(c-ctx), clampHi(c+ctx, n)
		if lo <= cur.hi+1 {
			if hi > cur.hi {
				cur.hi = hi
			}
		} else {
			spans = append(spans, cur)
			cur = span{lo, hi}
		}
	}
	spans = append(spans, cur)

	var hunks []events.Hunk
	for _, s := range spans {
		hunks = append(hunks, makeHunk(lines[s.lo:s.hi+1]))
	}
	return hunks
}

func makeHunk(seg []events.DiffLine) events.Hunk {
	h := events.Hunk{Lines: seg}
	for _, l := range seg {
		if l.Old > 0 {
			if h.OldStart == 0 {
				h.OldStart = l.Old
			}
			h.OldCount++
		}
		if l.New > 0 {
			if h.NewStart == 0 {
				h.NewStart = l.New
			}
			h.NewCount++
		}
	}
	return h
}

func clampLo(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func clampHi(v, n int) int {
	if v > n-1 {
		return n - 1
	}
	return v
}
