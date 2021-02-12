package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/infra/pb"
)

func init() {
	tagpush.Flags().String("token", "", "User token.")
	tagpush.MarkFlagRequired("token")
	tagpush.Flags().String("input", "", "Input file (where to store the tag from).")
	tagpush.MarkFlagRequired("input")
	tagpush.Flags().String("url", "", "The URL of a tagger instance.")
	tagpush.MarkFlagRequired("url")
}

// pushParams gather all parameters needed to push a tag from a local
// file into a tagger instance.
type pushParams struct {
	url       string
	srcfile   string
	token     string
	namespace string
	name      string
}

var tagpush = &cobra.Command{
	Use:   "push <image tag>",
	Short: "Pushes a tag from a tar file and into a tagger instance",
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}
		name := args[0]

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

		return pushTag(
			pushParams{
				url:       url,
				srcfile:   srcfile,
				token:     token,
				namespace: namespace,
				name:      name,
			},
		)
	},
}

// pushTag issues a grpc call against a remote instance of tagger (pointed by
// url) and uploads a tar file (pointed by srcfile).
func pushTag(params pushParams) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	finfo, err := os.Stat(params.srcfile)
	if err != nil {
		return nil
	}

	fp, err := os.Open(params.srcfile)
	if err != nil {
		return err
	}
	defer fp.Close()

	// XXX implement ssl please?
	conn, err := grpc.Dial(params.url, grpc.WithInsecure())
	if err != nil {
		return err
	}

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Push(ctx)
	if err != nil {
		return err
	}

	ireq := &pb.PushRequest{
		TestOneof: &pb.PushRequest_Request{
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

	p := mpb.New(mpb.WithWidth(60))
	defer p.Wait()

	pbar := p.Add(
		finfo.Size(),
		mpb.NewBarFiller(" ▮▮▯ "),
		mpb.PrependDecorators(decor.Name("Uploading")),
		mpb.AppendDecorators(decor.CountersKiloByte("%d %d")),
	)

	content := make([]byte, 1024)
	for {
		read, err := fp.Read(content)
		if err == io.EOF {
			if _, err := stream.CloseAndRecv(); err != nil {
				return err
			}
			break
		} else if err != nil {
			return err
		}

		// Sends a chunk of the file.
		ireq := &pb.PushRequest{
			TestOneof: &pb.PushRequest_Chunk{
				Chunk: &pb.Chunk{
					Content: content,
				},
			},
		}
		if err := stream.Send(ireq); err != nil {
			return err
		}
		pbar.IncrBy(read)
	}
	return nil
}
