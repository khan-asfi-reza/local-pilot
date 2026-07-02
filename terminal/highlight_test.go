package main

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	cases := []struct{ in, want string }{
		{`def add(a, b):`, "[fuchsia]def[-]"},
		{`x = "hi"`, `[olive]"hi"[-]`},
		{`n = 42`, "[aqua]42[-]"},
		{`return x  # done`, "[gray]# done[-]"},
	}
	for _, c := range cases {
		got := highlight(c.in)
		if !strings.Contains(got, c.want) {
			t.Fatalf("highlight(%q) = %q, missing %q", c.in, got, c.want)
		}
	}
	// tview markup in code must be escaped, not treated as a color tag.
	if out := highlight(`items[0]`); !strings.Contains(out, "[[") {
		t.Fatalf("bracket not escaped: %q", out)
	}
}
