// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package static

import (
	_ "embed"
)

// Well, this file is full of lint complains but I don't care.

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

// Text is a map to all embed text files, indexed by their respective
// path relative to "src" directory.
var Text = map[string]string{
	"pull_help_header":     pull_help_header,
	"pull_help_examples":   pull_help_examples,
	"push_help_header":     push_help_header,
	"push_help_examples":   push_help_examples,
	"import_help_header":   import_help_header,
	"import_help_examples": import_help_examples,
}
