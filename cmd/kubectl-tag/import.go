package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ricardomaraschini/tagger/infra/pb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func init() {
	tagimport.Flags().String("token", "", "User token.")
	tagimport.MarkFlagRequired("token")
	tagimport.Flags().String("input", "", "Input file (where to store the tag from).")
	tagimport.MarkFlagRequired("input")
	tagimport.Flags().String("url", "", "The URL of a tagger instance.")
	tagimport.MarkFlagRequired("url")
}

type importParams struct {
	url       string
	srcfile   string
	token     string
	namespace string
	name      string
}

var tagimport = &cobra.Command{
	Use:   "import <image tag>",
	Short: "Imports a tag from a local tar file",
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

		srcfile, err := c.Flags().GetString("input")
		if err != nil {
			return err
		}

		namespace, err := Namespace(c)
		if err != nil {
			return err
		}

		return importTag(
			importParams{
				url:       url,
				srcfile:   srcfile,
				token:     token,
				namespace: namespace,
				name:      args[0],
			},
		)
	},
}

func importTag(params importParams) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fp, err := os.Open(params.srcfile)
	if err != nil {
		return err
	}
	defer fp.Close()

	conn, err := grpc.Dial(params.url, grpc.WithInsecure())
	if err != nil {
		return err
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Import(ctx)
	if err != nil {
		return err
	}

	ireq := &pb.ImportRequest{
		TestOneof: &pb.ImportRequest_Request{
			Request: &pb.Request{
				Name:      params.name,
				Namespace: params.namespace,
				Token:     params.token,
			},
		},
	}
	if err := stream.Send(ireq); err != nil {
		return err
	}

	for {
		content := make([]byte, 2*1024*1024)
		if _, err := fp.Read(content); err != nil {
			if err == io.EOF {
				if _, err := stream.CloseAndRecv(); err != nil {
					return err
				}
				break
			}
			return err
		}

		ireq := &pb.ImportRequest{
			TestOneof: &pb.ImportRequest_Chunk{
				Chunk: &pb.Chunk{
					Content: content,
				},
			},
		}
		if err := stream.Send(ireq); err != nil {
			return err
		}
	}
	return nil
}
