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
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagimport.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagimport = &cobra.Command{
	Use:     "import <image tag>",
	Short:   "Imports a new generation for a tag",
	Long:    static.Text["import_help_header"],
	Example: static.Text["import_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatal("provide an image tag")
		}

		svc, err := createTagService()
		if err != nil {
			log.Fatal(err)
		}

		ns, err := namespace(c)
		if err != nil {
			log.Fatal(err)
		}

		it, err := svc.NewGeneration(c.Context(), ns, args[0])
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("tag %s imported (gen %d)", args[0], it.Spec.Generation)
	},
}
