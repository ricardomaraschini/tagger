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
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
	"github.com/ricardomaraschini/tagger/infra/progbar"
)

var tagpush = &cobra.Command{
	Use:     "push <tagger.instance:port/namespace/name>",
	Short:   "Pushes an image into the next generation of a tag",
	Long:    static.Text["push_help_header"],
	Example: static.Text["push_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatal("invalid number of arguments")
		}

		config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			log.Fatal(err)
		}

		if config.BearerToken == "" {
			log.Fatal("empty token, you need a kubernetes token to push")
		}

		// first understands what tag is the user referring to.
		tidx, err := indexFor(args[0])
		if err != nil {
			log.Fatal(err)
		}

		// now we save the image from the local storage and into
		// a tar file.
		srcref, cleanup, err := saveTagImage(c.Context(), tidx)
		if err != nil {
			log.Fatal(err)
		}
		defer cleanup()

		if err := pushTagImage(
			c.Context(), tidx, srcref, config.BearerToken,
		); err != nil {
			log.Fatal(err)
		}
	},
}

// saveTagImage saves an image present in the local storage into a local
// tar file. This function returns a "cleanup" func that must be called
// to release used resources.
func saveTagImage(ctx context.Context, tidx tagindex) (*os.File, func(), error) {
	fsh := fs.New("")
	fp, cleanup, err := fsh.TempFile()
	if err != nil {
		return nil, nil, err
	}

	str := fmt.Sprintf("docker-archive:%s", fp.Name())
	dstref, err := alltransports.ParseImageName(str)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	srcref, err := tidx.localStorageRef()
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	pol := &signature.Policy{
		Default: signature.PolicyRequirements{
			signature.NewPRInsecureAcceptAnything(),
		},
	}
	pctx, err := signature.NewPolicyContext(pol)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	if _, err := imgcopy.Image(
		ctx, pctx, dstref, srcref, &imgcopy.Options{},
	); err != nil {
		cleanup()
		return nil, nil, err
	}
	return fp, cleanup, err
}

// pushTagImages sends an image through GRPC to a tagger instance.
func pushTagImage(
	ctx context.Context, idx tagindex, from *os.File, token string,
) error {
	// XXX implement ssl please
	conn, err := grpc.Dial(idx.server, grpc.WithInsecure())
	if err != nil {
		return err
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Push(ctx)
	if err != nil {
		return err
	}

	// we first send over a communication to indicate we are
	// willing to send an image. That will bail out if the
	// provided info is wrong.
	header := &pb.Header{
		Name:      idx.name,
		Namespace: idx.namespace,
		Token:     token,
	}

	if err := stream.Send(
		&pb.Packet{
			TestOneof: &pb.Packet_Header{
				Header: header,
			},
		},
	); err != nil {
		return err
	}

	finfo, err := from.Stat()
	if err != nil {
		return err
	}
	fsize := finfo.Size()

	pbar := progbar.New(ctx, "Pushing")
	pbar.SetMax(fsize)
	defer pbar.Wait()

	if err := pb.Send(from, fsize, stream, pbar); err != nil {
		return err
	}

	_, err = stream.CloseAndRecv()
	return err
}
