package main

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/ricardomaraschini/tagger/imagetags/pb"
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
	stream, err := client.Export(ctx, &pb.Request{
		Name:      "simple-web-server",
		Namespace: "rmarasch",
		Token:     "sha256~DsU0l3HqCA5kF761bID0m52FZ1i9ZPzph9F-o1Y8wAs",
	})
	if err != nil {
		log.Fatal("Export: ", err)
	}

	fp, err := os.Create("/tmp/badanha.tar.gz")
	if err != nil {
		log.Fatalf("error creating file: %s", err)
	}
	defer fp.Close()
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Fatalf("cannot receive %v", err)
		}

		if _, err := fp.Write(resp.Content); err != nil {
			log.Fatalf("Write: %s", err)
		}
		log.Printf("Resp received: %d\n", len(resp.Content))
	}
}
