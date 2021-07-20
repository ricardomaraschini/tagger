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

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagmirror.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagmirror = &cobra.Command{
	Use:     "mirror registry.io/repo/name:tag -n namespace tagname",
	Short:   "Mirrors a remote image into a tag",
	Long:    static.Text["mirror_help_header"],
	Example: static.Text["mirror_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("provide an image source and a tag name")
		}

		svc, err := createTagService()
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		return svc.NewTag(c.Context(), ns, args[1], args[0], true)
	},
}
