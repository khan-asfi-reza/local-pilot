// Package localpilot embeds the default config and skills so the binary can seed
// a fresh data directory and run from anywhere, with no repo checkout needed.
package localpilot

import "embed"

//go:embed models/models.json models/prompt.json skills
var Defaults embed.FS
