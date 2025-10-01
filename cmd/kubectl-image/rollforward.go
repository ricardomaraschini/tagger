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
)

func init() {
	imagerollforward.Flags().StringP("namespace", "n", "", "namespace to use")
}

var imagerollforward = &cobra.Command{
	Use:   "forward <image name>",
	Short: "Rolls forward to next image version",
	RunE: func(c *cobra.Command, args []string) error {
		ctx := c.Context()
		if len(args) != 1 {
			return fmt.Errorf("invalid number of arguments")
		}

		svc, err := createImageService(ctx)
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		img, err := svc.Get(ctx, ns, args[0])
		if err != nil {
			return err
		}

		return svc.Rollforward(ctx, img)
	},
}
