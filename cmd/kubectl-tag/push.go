package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

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

		if err := pushTagImage(tidx, srcref, config.BearerToken); err != nil {
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

	pbar := progbar.New("Pushing")
	defer pbar.Wait()

	if err := pb.Send(from, stream, pbar); err != nil {
		return err
	}

	_, err = stream.CloseAndRecv()
	return err
}
