package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	imgcopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func init() {
	tagpull.Flags().String("token", "", "User token.")
	tagpull.MarkFlagRequired("token")
}

func splitDomainRepoAndName(imgPath string) (string, string, string) {
	slices := strings.SplitN(imgPath, "/", 3)
	if len(slices) < 3 {
		return "", "", ""
	}
	url := slices[0]
	repo := slices[1]
	remainder := slices[2]

	slices = strings.SplitN(remainder, ":", 2)
	if len(slices) == 2 {
		return url, repo, slices[0]
	}

	slices = strings.SplitN(remainder, "@", 2)
	if len(slices) == 2 {
		return url, repo, slices[0]
	}

	return url, repo, remainder
}

var tagpull = &cobra.Command{
	Use:   "pull <image>",
	Short: "Pull a tag image from a tagger instance",
	Run: func(c *cobra.Command, args []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if len(args) != 1 {
			log.Fatalf("invalid command, missing tag name")
		}

		token, err := c.Flags().GetString("token")
		if err != nil {
			log.Fatal("token flag not valid")
		}

		url, namespace, name := splitDomainRepoAndName(args[0])
		if url == "" {
			log.Fatalf("invalid image reference: %s", args[0])
		}

		// XXX ssl please?
		conn, err := grpc.Dial(url, grpc.WithInsecure())
		if err != nil {
			log.Fatalf("error connecting to tagger: %s", err)
		}

		client := pb.NewTagIOServiceClient(conn)
		stream, err := client.Pull(ctx, &pb.Request{
			Name:      name,
			Namespace: namespace,
			Token:     token,
		})
		if err != nil {
			log.Fatalf("error sending request: %s", err)
		}

		fsh := fs.New("")
		fp, cleanup, err := fsh.TempFile()
		if err != nil {
			log.Fatalf("error creating temp file: %s", err)
		}
		defer cleanup()

		if _, err := pb.ReceiveFileClient(fp, stream); err != nil {
			log.Fatalf("errror receiving file: %s", err)
		}

		srcref := fmt.Sprintf("docker-archive:%s", fp.Name())
		fromRef, err := alltransports.ParseImageName(srcref)
		if err != nil {
			log.Fatalf("errror parsing local reference: %s", err)
		}

		dstpath := fmt.Sprintf(
			"containers-storage:%s/%s/%s:latest",
			url, namespace, name,
		)

		toRef, err := alltransports.ParseImageName(dstpath)
		if err != nil {
			log.Fatalf("errror parsing storage reference: %s", err)
		}

		pol := &signature.Policy{
			Default: signature.PolicyRequirements{
				signature.NewPRInsecureAcceptAnything(),
			},
		}
		polctx, err := signature.NewPolicyContext(pol)
		if err != nil {
			log.Fatalf("errror creating policy context: %s", err)
		}

		if _, err := imgcopy.Image(
			ctx, polctx, toRef, fromRef, &imgcopy.Options{},
		); err != nil {
			log.Fatalf("error copying image: %s", err)
		}
	},
}
