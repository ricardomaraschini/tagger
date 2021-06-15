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
	"log"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
	"github.com/ricardomaraschini/tagger/infra/progbar"
)

var tagpull = &cobra.Command{
	Use:     "pull <tagger.instance:port/namespace/name>",
	Short:   "Pulls current Tag image",
	Long:    static.Text["pull_help_header"],
	Example: static.Text["pull_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatal("invalid number of arguments")
		}

		config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			log.Fatal(err)
		}

		if config.BearerToken == "" {
			log.Fatal("empty token, you need a kubernetes token to pull")
		}

		// first understands what tag is the user referring to.
		tidx, err := indexFor(args[0])
		if err != nil {
			log.Fatal(err)
		}

		// now that we know what is the tag we do the grpc call
		// to retrieve the image. The output here is a local tar
		// file from where we can load the image into runtime's
		// local storage.
		srcref, cleanup, err := pullTagImage(
			c.Context(), tidx, config.BearerToken,
		)
		if err != nil {
			log.Fatal(err)
		}
		defer cleanup()

		dstref, err := tidx.localStorageRef()
		if err != nil {
			log.Fatal(err)
		}

		pol := &signature.Policy{
			Default: signature.PolicyRequirements{
				signature.NewPRInsecureAcceptAnything(),
			},
		}
		polctx, err := signature.NewPolicyContext(pol)
		if err != nil {
			log.Fatal(err)
		}

		// copy the image into runtime's local storage.
		if _, err := imgcopy.Image(
			c.Context(), polctx, dstref, srcref, &imgcopy.Options{},
		); err != nil {
			log.Fatal(err)
		}
	},
}

// pullTagImage pulls the current generation for a tag identified by tagindex.
// Returns a reference to the locally stored image (on disk) and a function to
// be called at the end to clean up our mess. If this function returns an error
// then callers don't need to call the clean-up function.
func pullTagImage(
	ctx context.Context, idx tagindex, token string,
) (types.ImageReference, func(), error) {
	// XXX ssl goes here, please.
	conn, err := grpc.Dial(idx.server, grpc.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting: %w", err)
	}

	header := &pb.Header{
		Name:      idx.name,
		Namespace: idx.namespace,
		Token:     token,
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Pull(
		ctx,
		&pb.Packet{
			TestOneof: &pb.Packet_Header{
				Header: header,
			},
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error pulling: %w", err)
	}

	fsh := fs.New("")
	fp, cleanup, err := fsh.TempFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temp file: %w", err)
	}

	pbar := progbar.New(ctx, "Pulling")
	defer pbar.Wait()

	if err := pb.Receive(stream, fp, pbar); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error receiving file: %w", err)
	}

	str := fmt.Sprintf("docker-archive:%s", fp.Name())
	fromref, err := alltransports.ParseImageName(str)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error parsing reference: %w", err)
	}

	return fromref, cleanup, nil
}
