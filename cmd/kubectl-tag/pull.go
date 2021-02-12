package main

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/infra/pb"
)

func init() {
	tagpull.Flags().String("token", "", "User token.")
	tagpull.MarkFlagRequired("token")
	tagpull.Flags().String("output", "", "Output file (where to store the tag).")
	tagpull.MarkFlagRequired("output")
	tagpull.Flags().String("url", "", "The URL of a tagger instance.")
	tagpull.MarkFlagRequired("url")
}

// pullParams gather all parameters needed to pull a tag from a tagger
// instance into a local file.
type pullParams struct {
	url       string
	dstfile   string
	token     string
	namespace string
	name      string
}

var tagpull = &cobra.Command{
	Use:   "pull <image tag>",
	Short: "Pull a tag from a tagger instance and into a tar file",
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatalf("invalid command, missing tag name")
		}
		name := args[0]

		url, err := c.Flags().GetString("url")
		if err != nil {
			return
		}

		token, err := c.Flags().GetString("token")
		if err != nil {
			return
		}

		dstfile, err := c.Flags().GetString("output")
		if err != nil {
			return
		}

		namespace, err := Namespace(c)
		if err != nil {
			log.Fatalf("error determining current namespace: %s", err)
			return
		}

		if err := pullTag(
			pullParams{
				url:       url,
				dstfile:   dstfile,
				token:     token,
				namespace: namespace,
				name:      name,
			},
		); err != nil {
			log.Fatalf("error pulling tag: %s", err)
		}
	},
}

// pullTag does a grpc call to the remote tagger instance, awaits for the tag
// to be exported and retrieves it. Content is written to params.dstfile.
func pullTag(params pullParams) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fp, err := os.Create(params.dstfile)
	if err != nil {
		return err
	}
	defer fp.Close()

	// XXX ssl please?
	conn, err := grpc.Dial(params.url, grpc.WithInsecure())
	if err != nil {
		return err
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Pull(ctx, &pb.Request{
		Name:      params.name,
		Namespace: params.namespace,
		Token:     params.token,
	})
	if err != nil {
		return err
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if _, err := fp.Write(resp.Content); err != nil {
			return err
		}
	}
	return nil
}
