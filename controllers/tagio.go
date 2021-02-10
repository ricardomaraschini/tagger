package controllers

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"k8s.io/klog/v2"

	"github.com/ricardomaraschini/tagger/imagetags/pb"
)

// TagImporterExporter is here to make tests easier. You may be looking for
// its concrete implementation in services/tagio.go.
type TagImporterExporter interface {
	Import(context.Context, string, string, io.Reader) error
	Export(context.Context, string, string) (io.ReadCloser, func(), error)
}

// UserValidator validates an user can access Tags in a given namespace.
// You should be looking for a concrete implementation of this, please
// look at services/user.go and you will find it.
type UserValidator interface {
	CanAccessTags(context.Context, string, string) error
}

// TagIO handles requests for exporting and import Tag custom resources.
type TagIO struct {
	bind   string
	tagexp TagImporterExporter
	usrval UserValidator
	srv    *grpc.Server
	pb.UnimplementedTagIOServiceServer
}

// NewTagIO returns a web hook handler for quay webhooks.
func NewTagIO(
	tagexp TagImporterExporter,
	usrval UserValidator,
) *TagIO {
	tio := &TagIO{
		bind:   ":8083",
		tagexp: tagexp,
		usrval: usrval,
		srv:    grpc.NewServer(),
	}
	pb.RegisterTagIOServiceServer(tio.srv, tio)
	reflection.Register(tio.srv)
	return tio
}

// Export handles tag exports through grpc.
func (t *TagIO) Export(in *pb.Request, stream pb.TagIOService_ExportServer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := t.usrval.CanAccessTags(
		ctx, in.GetNamespace(), in.GetToken(),
	); err != nil {
		klog.Errorf("user cannot access tags in namespace: %s", err)
		return fmt.Errorf("unauthorized")
	}

	fp, cleanup, err := t.tagexp.Export(
		ctx, in.GetNamespace(), in.GetName(),
	)
	if err != nil {
		return fmt.Errorf("error importing image: %w", err)
	}
	defer cleanup()

	chunk := &pb.Chunk{
		Content: make([]byte, 256),
	}
	for {
		if _, err := fp.Read(chunk.Content); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading tag file: %w", err)
		}
		if err := stream.Send(chunk); err != nil {
			return fmt.Errorf("error sending blob: %w", err)
		}
	}
	return nil
}

// Import handles tag imports through grpc.
func (t *TagIO) Import(ctx context.Context, in *pb.ImportRequest) (*pb.ImportResult, error) {
	return &pb.ImportResult{}, nil
	/*
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := t.usrval.CanAccessTags(
			ctx, in.GetNamespace(), in.GetToken(),
		); err != nil {
			klog.Errorf("user cannot access tags in namespace: %s", err)
			return nil, fmt.Errorf("unauthorized")
		}

		buf := bytes.NewBuffer(nil)
		if err := t.tagexp.Import(
			ctx, in.GetNamespace(), in.GetName(), buf,
		); err != nil {
			klog.Errorf("error importing image: %s", err)
			return nil, fmt.Errorf("error importing image: %w", err)
		}

		return &pb.ImportResult{}, nil
	*/
}

// Name returns a name identifier for this controller.
func (t *TagIO) Name() string {
	return "tag input/output handler"
}

// Start puts the http server online.
func (t *TagIO) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", t.bind)
	if err != nil {
		return fmt.Errorf("error creating grpc socket: %w", err)
	}

	go func() {
		<-ctx.Done()
		t.srv.GracefulStop()
	}()

	return t.srv.Serve(listener)
}
