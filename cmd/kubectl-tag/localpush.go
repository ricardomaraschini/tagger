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
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	taglocalpush.Flags().Bool("insecure", false, "don't verify certificate when connecting")
	taglocalpush.Flags().String("file", "", "docker-archive file path")
}

var taglocalpush = &cobra.Command{
	Use:     "localpush --file=/path/docker-archive.tar <tagger.instance:port/namespace/name>",
	Short:   "Pushes a locally saved image (tar file) as the next generation for a tag",
	Long:    static.Text["localpush_help_header"],
	Example: static.Text["localpush_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("invalid number of arguments")
		}

		insecure, err := c.Flags().GetBool("insecure")
		if err != nil {
			return err
		}

		file, err := c.Flags().GetString("file")
		if err != nil {
			return err
		}
		if len(file) == 0 {
			return fmt.Errorf("invalid empty path to docker-archive")
		}

		config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			return err
		}

		if config.BearerToken == "" {
			return fmt.Errorf("empty token, you need a kubernetes token to push")
		}

		srcref, err := os.Open(file)
		if err != nil {
			return err
		}
		defer srcref.Close()

		tidx, err := indexFor(args[0])
		if err != nil {
			return err
		}

		return pushTagImage(c.Context(), tidx, srcref, config.BearerToken, insecure)
	},
}
