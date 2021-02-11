package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/infra/pb"
)

func init() {
	tagexport.Flags().String("token", "", "User token.")
	tagexport.MarkFlagRequired("token")
	tagexport.Flags().String("output", "", "Output file (where to store the tag).")
	tagexport.MarkFlagRequired("output")
	tagexport.Flags().String("url", "", "The URL of a tagger instance.")
	tagexport.MarkFlagRequired("url")
}

type exportParams struct {
	url       string
	dstfile   string
	token     string
	namespace string
	name      string
}

var tagexport = &cobra.Command{
	Use:   "export <image tag>",
	Short: "Exports a tag into a local tar file",
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}

		url, err := c.Flags().GetString("url")
		if err != nil {
			return err
		}

		token, err := c.Flags().GetString("token")
		if err != nil {
			return err
		}

		dstfile, err := c.Flags().GetString("output")
		if err != nil {
			return err
		}

		namespace, err := Namespace(c)
		if err != nil {
			return err
		}

		return exportTag(
			exportParams{
				url:       url,
				dstfile:   dstfile,
				token:     token,
				namespace: namespace,
				name:      args[0],
			},
		)
	},
}

func exportTag(params exportParams) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fp, err := os.Create(params.dstfile)
	if err != nil {
		return err
	}
	defer fp.Close()

	conn, err := grpc.Dial(params.url, grpc.WithInsecure())
	if err != nil {
		return err
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Export(ctx, &pb.Request{
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
