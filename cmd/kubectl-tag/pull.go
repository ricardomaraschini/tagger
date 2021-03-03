package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
)

var tagpull = &cobra.Command{
	Use:     "pull <tagger.instance:port/namespace/name>",
	Short:   "Pulls current Tag image",
	Long:    static.Text["pull_help_header"],
	Example: static.Text["pull_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("invalid number of arguments")
		}

		config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			return err
		}

		if config.BearerToken == "" {
			return fmt.Errorf("empty token, you need a kubernetes token to pull")
		}

		// first understands what tag is the user referring to.
		tidx, err := indexFor(args[0])
		if err != nil {
			return err
		}

		// now that we know what is the tag we do the grpc call
		// to retrieve the image and host it locally on disk.
		srcref, cleanup, err := pullTagImage(tidx, config.BearerToken)
		if err != nil {
			return err
		}
		defer cleanup()

		// now we need to understand to where we are copying this
		// image. we are copying to the local storage so just
		// grab a reference to it.
		dstref, err := tidx.localref()
		if err != nil {
			return err
		}

		pol := &signature.Policy{
			Default: signature.PolicyRequirements{
				signature.NewPRInsecureAcceptAnything(),
			},
		}
		polctx, err := signature.NewPolicyContext(pol)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// copy the image into local storage.
		_, err = imgcopy.Image(
			ctx, polctx, dstref, srcref, &imgcopy.Options{},
		)
		return err
	},
}

// pullTagImage pulls the current generation for a tag identified by tagindex.
// Returns a function to be called at the end to clean up resources.
func pullTagImage(idx tagindex, token string) (types.ImageReference, func(), error) {
	// XXX ssl goes here, please.
	conn, err := grpc.Dial(idx.server, grpc.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Pull(ctx, &pb.Request{
		Name:      idx.name,
		Namespace: idx.namespace,
		Token:     token,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error pulling: %w", err)
	}

	fsh := fs.New("")
	fp, cleanup, err := fsh.TempFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temp file: %w", err)
	}

	if _, err := pb.ReceiveFileClient(fp, stream); err != nil {
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

// localReferenceForTag returns what should be used when copying an image
// tag into local storage.
func localReferenceForTag(tidx tagindex) (types.ImageReference, error) {
	str := fmt.Sprintf(
		"containers-storage:%s/%s/%s:latest",
		tidx.server, tidx.namespace, tidx.name,
	)
	return alltransports.ParseImageName(str)
}
