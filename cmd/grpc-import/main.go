package main

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/ricardomaraschini/tagger/infra/pb"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial(
		"a7c8d6dc4c9e44e198ff07d42f03ec4f-1307792165.us-east-1.elb.amazonaws.com:8083",
		grpc.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("can not connect with server %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client := pb.NewTagIOServiceClient(conn)
	stream, err := client.Import(ctx)
	if err != nil {
		log.Fatal("Export: ", err)
	}

	if err := stream.Send(
		&pb.ImportRequest{
			TestOneof: &pb.ImportRequest_Request{
				Request: &pb.Request{
					Name:      "simple-web-server-rmarasch",
					Namespace: "rmarasch",
					Token:     "sha256~DsU0l3HqCA5kF761bID0m52FZ1i9ZPzph9F-o1Y8wAs",
				},
			},
		},
	); err != nil {
		log.Fatalf("error sending initial message: %s", err)
	}

	fp, err := os.Open("/tmp/badanha.tar.gz")
	if err != nil {
		log.Fatalf("error creating file: %s", err)
	}
	defer fp.Close()

	for {
		content := make([]byte, 2*1024*1024)
		if _, err := fp.Read(content); err != nil {
			if err == io.EOF {
				if _, err := stream.CloseAndRecv(); err != nil {
					log.Fatalf("CloseAndRecv: %s", err)
				}
				break
			}
			log.Fatalf("error reading from tar: %s", err)
		}

		if err := stream.Send(
			&pb.ImportRequest{
				TestOneof: &pb.ImportRequest_Chunk{
					Chunk: &pb.Chunk{
						Content: content,
					},
				},
			},
		); err != nil {
			log.Fatalf("error sending chunk: %s", err)
		}
	}
}
