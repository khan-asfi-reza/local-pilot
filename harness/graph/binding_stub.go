//go:build nots

package graph

import "context"

// Enabled reports false in a pure-Go (CGO_ENABLED=0) build compiled with the
// `nots` tag: no tree-sitter, so callers use the regex repo map instead.
func Enabled() bool { return false }

type stubParser struct{}

func newParser() Parser { return stubParser{} }

func (stubParser) Supports(string) bool { return false }

func (stubParser) Parse(context.Context, string, string, []byte) ([]*Node, []RawRef, error) {
	return nil, nil, nil
}
