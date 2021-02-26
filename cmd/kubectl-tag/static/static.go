package static

import _ "embed"

//go:embed "src/pull_help_header"
var pull_help_header string

//go:embed "src/pull_help_examples"
var pull_help_examples string

//go:embed "src/push_help_header"
var push_help_header string

//go:embed "src/push_help_examples"
var push_help_examples string

// Text is a map to all embed text files, indexed by their respective
// path relative to "src" directory.
var Text = map[string]string{
	"pull_help_header":   pull_help_header,
	"pull_help_examples": pull_help_examples,
	"push_help_header":   push_help_header,
	"push_help_examples": push_help_examples,
}
