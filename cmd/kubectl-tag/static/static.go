package static

// Well, this file is full of lint complains but I don't care.

import (
	"embed"
)

//go:embed "src/pull_help_header"
var pull_help_header string

//go:embed "src/pull_help_examples"
var pull_help_examples string

//go:embed "src/push_help_header"
var push_help_header string

//go:embed "src/push_help_examples"
var push_help_examples string

//go:embed "src/import_help_header"
var import_help_header string

//go:embed "src/import_help_examples"
var import_help_examples string

//go:embed "src/upgrade_help_header"
var upgrade_help_header string

//go:embed "src/upgrade_help_examples"
var upgrade_help_examples string

//go:embed "src/downgrade_help_header"
var downgrade_help_header string

//go:embed "src/downgrade_help_examples"
var downgrade_help_examples string

//go:embed "src/new_help_header"
var new_help_header string

//go:embed "src/new_help_examples"
var new_help_examples string

//go:embed "src/mirror_help_header"
var mirror_help_header string

//go:embed "src/mirror_help_examples"
var mirror_help_examples string

//go:embed src/*
var FS embed.FS

// Text is a map to all embed text files, indexed by their respective
// path relative to "src" directory.
var Text = map[string]string{
	"pull_help_header":        pull_help_header,
	"pull_help_examples":      pull_help_examples,
	"push_help_header":        push_help_header,
	"push_help_examples":      push_help_examples,
	"import_help_header":      import_help_header,
	"import_help_examples":    import_help_examples,
	"upgrade_help_header":     upgrade_help_header,
	"upgrade_help_examples":   upgrade_help_examples,
	"downgrade_help_header":   downgrade_help_header,
	"downgrade_help_examples": downgrade_help_examples,
	"new_help_header":         new_help_header,
	"new_help_examples":       new_help_examples,
	"mirror_help_header":      mirror_help_header,
	"mirror_help_examples":    mirror_help_examples,
}
