package main

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
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

	var prevdescr string
	var pbar *mpb.Bar
	p := mpb.New(mpb.WithWidth(60))
	defer p.Wait()
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if progress := resp.GetProgress(); progress != nil {
			descr := formatDescription(progress.Description)
			if prevdescr != descr {
				prevdescr = descr
				pbar = p.Add(
					progress.Size,
					mpb.NewBarFiller(" ▮▮▯ "),
					mpb.PrependDecorators(decor.Name(descr)),
					mpb.AppendDecorators(
						decor.CountersKiloByte("%d %d"),
					),
				)
			}
			pbar.SetCurrent(int64(progress.Offset))
			continue
		}

		chunk := resp.GetChunk()
		if _, err := fp.Write(chunk.Content); err != nil {
			return err
		}
	}
	return nil
}

// formatDescription removes "sha256:" prefix of a description and makes it to have
// a 13 in length.
func formatDescription(descr string) string {
	descr = strings.TrimPrefix(descr, "sha256:")
	strlen := len(descr)

	switch {
	case strlen > 13:
		return descr[:13]
	case strlen < 13:
		return descr + strings.Repeat(" ", 13-strlen)
	default:
		return descr
	}
}
