package main

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/ricardomaraschini/tagger/imagetags/pb"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial(
		"tagio-tagger.apps.rmarasch-210210a.devcluster.openshift.com:8083",
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
		Token:     "sha256~pInBioup4LL5HPsM5m-HP_MvcfIgbLcvtM86V-OtDoI",
	})
	if err != nil {
		log.Fatal("Export: ", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Fatalf("cannot receive %v", err)
		}
		log.Printf("Resp received: %d\n", len(resp.Content))
	}
}
