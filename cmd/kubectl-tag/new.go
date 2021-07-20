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
	tagnew.Flags().StringP("namespace", "n", "", "namespace to use")
	tagnew.Flags().StringP("from", "f", "", "from where to import the tag")
	tagnew.Flags().Bool("mirror", false, "mirror the image")
	tagnew.MarkFlagRequired("from")
}

var tagnew = &cobra.Command{
	Use:     "new --from reg.io/repo/name:tag --mirror -n namespace tagname",
	Short:   "Creates a new tag by importing it from a remote registry",
	Long:    static.Text["new_help_header"],
	Example: static.Text["new_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag name")
		}

		svc, err := createTagService()
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		mirror, err := c.Flags().GetBool("mirror")
		if err != nil {
			return err
		}

		from, err := c.Flags().GetString("from")
		if err != nil {
			return err
		}

		return svc.NewTag(c.Context(), ns, args[0], from, mirror)
	},
}
