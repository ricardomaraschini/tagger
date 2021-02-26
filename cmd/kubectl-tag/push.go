package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
	"google.golang.org/grpc"

	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
)

func init() {
	tagpush.Flags().String("token", "", "User Kubernetes access token.")
	tagpush.MarkFlagRequired("token")
}

var tagpush = &cobra.Command{
	Use:   "push <image>",
	Short: "Pushes an image into tagger",
	RunE: func(c *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if len(args) != 1 {
			return fmt.Errorf("provide an image name")
		}

		url, namespace, name := splitDomainRepoAndName(args[0])
		if url == "" {
			log.Fatalf("invalid image reference: %s", args[0])
		}

		token, err := c.Flags().GetString("token")
		if err != nil {
			return err
		}

		fsh := fs.New("")
		fp, cleanup, err := fsh.TempFile()
		if err != nil {
			return err
		}
		defer cleanup()

		torefstr := fmt.Sprintf("docker-archive:%s", fp.Name())
		toref, err := alltransports.ParseImageName(torefstr)
		if err != nil {
			return err
		}

		fromrefstr := fmt.Sprintf("containers-storage:%s", args[0])
		fromref, err := alltransports.ParseImageName(fromrefstr)
		if err != nil {
			return err
		}

		pol := &signature.Policy{
			Default: signature.PolicyRequirements{
				signature.NewPRInsecureAcceptAnything(),
			},
		}
		pctx, err := signature.NewPolicyContext(pol)
		if err != nil {
			return err
		}

		if _, err := imgcopy.Image(
			ctx, pctx, toref, fromref, &imgcopy.Options{},
		); err != nil {
			return err
		}

		// XXX implement ssl please?
		conn, err := grpc.Dial(url, grpc.WithInsecure())
		if err != nil {
			return err
		}

		nfp, err := os.Open(fp.Name())
		if err != nil {
			return err
		}
		defer nfp.Close()

		finfo, err := nfp.Stat()
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
					Name:      name,
					Namespace: namespace,
					Token:     token,
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
			read, err := nfp.Read(content)
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

	},
}
