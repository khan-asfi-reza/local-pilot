package agent

import (
	"strings"
	"testing"
)

func TestNormalizeJSONUnstringifies(t *testing.T) {
	// A model that double-encodes the array (tasks as a JSON string).
	got := normalizeJSON(`{"tasks":"[{\"id\":\"t1\",\"title\":\"A\"}]"}`)
	want := `{"tasks":[{"id":"t1","title":"A"}]}`
	if got != want {
		t.Errorf("normalizeJSON = %s, want %s", got, want)
	}
	// Already-clean JSON is unchanged (modulo key order).
	if normalizeJSON(`{"a":1}`) != `{"a":1}` {
		t.Errorf("clean JSON altered")
	}
	// Non-JSON string values are left alone.
	if got := normalizeJSON(`{"name":"hello"}`); got != `{"name":"hello"}` {
		t.Errorf("plain string mangled: %s", got)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"here: {\"a\":1} done":    `{"a":1}`,
		"{\"a\":1}":               `{"a":1}`,
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsBigDocument(t *testing.T) {
	bigPRD := "# Title\n" + strings.Repeat("some spec line describing behavior in detail.\n", 40)

	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"long doc", bigPRD, true},
		{"many sections", "## A\n## B\n## C\n## D\nbuild it", true},
		{"numbered sections", "1. do x\n2. do y\n3. do z\n4. do w", true},
		{"single-file task", "remove comments from area.c", false},
		{"short prompt", "fix the bug", false},
	}
	for _, tc := range cases {
		if got := isBigDocument(tc.prompt); got != tc.want {
			t.Errorf("%s: isBigDocument = %v, want %v", tc.name, got, tc.want)
		}
	}
}
