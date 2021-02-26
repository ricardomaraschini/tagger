package main

import (
	"context"
	"fmt"
	"os"
	"time"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func init() {
	tagpush.Flags().String("token", "", "User Kubernetes access token.")
	tagpush.MarkFlagRequired("token")
}

// saveTagImage saves an image present in the local storage into a local
// tar file. This function returns a "cleanup" func that must be called
// to release used resources.
func saveTagImage(tidx tagindex) (*os.File, func(), error) {
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

	srcref, err := tidx.localref()
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if _, err := imgcopy.Image(
		ctx, pctx, dstref, srcref, &imgcopy.Options{},
	); err != nil {
		cleanup()
		return nil, nil, err
	}

	return fp, cleanup, err
}

// pushTagImages sends an image through GRPC to a tagger instance.
func pushTagImage(idx tagindex, from *os.File, token string) error {
	// XXX implement ssl please
	conn, err := grpc.Dial(idx.server, grpc.WithInsecure())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Push(ctx)
	if err != nil {
		return err
	}

	// we first send over a communication to indicate we are
	// willing to send an image. That will bail out if the
	// provided info is wrong.
	ireq := &pb.PushRequest{
		TestOneof: &pb.PushRequest_Request{
			Request: &pb.Request{
				Name:      idx.name,
				Namespace: idx.namespace,
				Token:     token,
			},
		},
	}
	if err := stream.Send(ireq); err != nil {
		return err
	}

	_, err = pb.SendFileClient(from, stream)
	return err
}

var tagpush = &cobra.Command{
	Use:     "push <tagger.instance:port/namespace/name>",
	Short:   "Pushes an image into the next generation of a tag",
	Long:    static.Text["push_help_header"],
	Example: static.Text["push_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("invalid number of arguments")
		}

		token, err := c.Flags().GetString("token")
		if err != nil {
			return err
		}

		// first understands what tag is the user referring to.
		tidx, err := indexFor(args[0])
		if err != nil {
			return err
		}

		// now we save the image from the local storage and into
		// a tar file.

		srcref, cleanup, err := saveTagImage(tidx)
		if err != nil {
			return err
		}
		defer cleanup()

		return pushTagImage(tidx, srcref, token)
	},
}
