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
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagdowngrade.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagdowngrade = &cobra.Command{
	Use:     "downgrade <image tag>",
	Short:   "Moves a tag to an older generation",
	Long:    static.Text["downgrade_help_header"],
	Example: static.Text["downgrade_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}

		svc, err := createTagService()
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		it, err := svc.Downgrade(c.Context(), ns, args[0])
		if err != nil {
			return err
		}

		log.Printf("tag %s downgraded (gen %d)", args[0], it.Spec.Generation)
		return nil
	},
}
