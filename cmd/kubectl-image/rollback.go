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
	"context"
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/spf13/cobra"

	itagcli "tagger/infra/images/v1beta1/gen/clientset/versioned"
	"tagger/services"
)

func init() {
	imagerollback.Flags().StringP("namespace", "n", "", "namespace to use")
}

var imagerollback = &cobra.Command{
	Use:   "back <image name>",
	Short: "Rolls an image back to its previous hash reference",
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

		return svc.Rollback(ctx, img)
	},
}

// createImageService creates an image service providing tooling to work with
// image objects. this is not a complete object, not all properties are set.
func createImageService(ctx context.Context) (*services.Image, error) {
	cfgpath := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", cfgpath)
	if err != nil {
		return nil, fmt.Errorf("error building config: %s", err)
	}

	imgcli, err := itagcli.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return services.NewImage(nil, imgcli, nil), nil
}
