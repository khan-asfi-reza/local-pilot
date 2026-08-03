//go:build !cgo

package graph

import "context"

// Enabled reports false in a pure-Go build (CGO disabled, e.g. Windows without a
// C compiler): no tree-sitter, so callers use the regex repo map instead. The
// split is on the `cgo` build tag, so the correct implementation is chosen
// automatically from whether CGO is available — no manual build flag needed.
func Enabled() bool { return false }

type stubParser struct{}

func newParser() Parser { return stubParser{} }

func (stubParser) Supports(string) bool { return false }

func (stubParser) Parse(context.Context, string, string, []byte) ([]*Node, []RawRef, error) {
	return nil, nil, nil
}
