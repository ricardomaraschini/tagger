package controllers

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"k8s.io/klog/v2"

	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
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
	fs     *fs.FS
	pb.UnimplementedTagIOServiceServer
}

// NewTagIO returns a web hook handler for quay webhooks. We have hardcoded
// what seems to be a reasonable value in terms of keep alive and connection
// lifespan management (we may need to better tune this).
func NewTagIO(
	tagexp TagImporterExporter,
	usrval UserValidator,
) *TagIO {
	aliveopt := grpc.KeepaliveParams(
		keepalive.ServerParameters{
			MaxConnectionIdle:     time.Minute,
			MaxConnectionAge:      10 * time.Minute,
			MaxConnectionAgeGrace: 10 * time.Second,
			Time:                  time.Second,
			Timeout:               2 * time.Second,
		},
	)
	tio := &TagIO{
		bind:   ":8083",
		tagexp: tagexp,
		usrval: usrval,
		fs:     fs.New("/data"),
		srv:    grpc.NewServer(aliveopt),
	}
	pb.RegisterTagIOServiceServer(tio.srv, tio)
	reflection.Register(tio.srv)
	return tio
}

// Export handles tag exports through grpc. We receive a request informing what
// is the tag to be exported (namespace and name) and also a kubernetes token
// for authentication and authorization. This function writes down the exported
// tag file in Chunks, client can then reassemble the file downstream.
func (t *TagIO) Export(in *pb.Request, stream pb.TagIOService_ExportServer) error {
	ctx := stream.Context()
	klog.Info("received request to export tag")

	if err := t.validateRequest(in); err != nil {
		klog.Errorf("error validating export request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	name := in.GetName()
	namespace := in.GetNamespace()
	token := in.GetToken()
	if err := t.usrval.CanAccessTags(ctx, namespace, token); err != nil {
		klog.Errorf("user cannot access tags in namespace: %s", err)
		return fmt.Errorf("unauthorized")
	}

	klog.Infof("exporting tag %s/%s", namespace, name)
	fp, cleanup, err := t.tagexp.Export(ctx, namespace, name)
	if err != nil {
		klog.Errorf("error exporting tag: %s", err)
		return fmt.Errorf("error exporting tag: %w", err)
	}
	defer cleanup()

	// each chunk is arbitrarily defined to be of 2MB in size.
	content := make([]byte, 2*1024*1024)
	chunk := &pb.Chunk{Content: content}
	for {
		if _, err := fp.Read(chunk.Content); err != nil {
			if err == io.EOF {
				break
			}
			klog.Errorf("error reading tag file: %s", err)
			return fmt.Errorf("error reading tag file: %w", err)
		}
		if err := stream.Send(chunk); err != nil {
			klog.Errorf("error sending blob: %s", err)
			return fmt.Errorf("error sending blob: %w", err)
		}
	}

	klog.Infof("tag %s/%s exported successfully", namespace, name)
	return nil
}

// validateRequest checks if all mandatory fields in a request are present.
func (t *TagIO) validateRequest(req *pb.Request) error {
	if req.GetName() == "" {
		return fmt.Errorf("empty name field")
	}
	if req.GetNamespace() == "" {
		return fmt.Errorf("empty namespace field")
	}
	if req.GetToken() == "" {
		return fmt.Errorf("empty token field")
	}
	return nil
}

// Import handles tag imports through grpc. The first message received indicates
// the destination for the tag (namespace and name) and a authorization token,
// all subsequent messages are of type Chunk where we can find a slice of bytes.
// By gluing Chunks together we have the tag tar file then we can call Import
// passing the file as parameter.
func (t *TagIO) Import(stream pb.TagIOService_ImportServer) error {
	ctx := stream.Context()
	klog.Info("received request to import tag")

	tmpfile, cleanup, err := t.fs.TempFile()
	if err != nil {
		klog.Errorf("error creating temp file: %s", err)
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer cleanup()

	in, err := stream.Recv()
	if err != nil {
		klog.Errorf("error receiving import request: %s", err)
		return fmt.Errorf("error receiving import request: %w", err)
	}

	req := in.GetRequest()
	if req == nil {
		klog.Errorf("first message of invalid type chunk")
		return fmt.Errorf("first message of invalid type chunk")
	}

	if err := t.validateRequest(req); err != nil {
		klog.Errorf("error validating export request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	name := req.GetName()
	namespace := req.GetNamespace()
	token := req.GetToken()
	klog.Infof("output tag set to %s/%s", namespace, name)
	if err := t.usrval.CanAccessTags(ctx, namespace, token); err != nil {
		klog.Errorf("user cannot access tags in namespace: %s", err)
		return fmt.Errorf("unauthorized")
	}

	var fsize int64
	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			klog.Errorf("error receiving chunk: %s", err)
			return fmt.Errorf("error receiving chunk: %w", err)
		}

		ck := in.GetChunk()
		if ck == nil {
			klog.Error("nil chunk received")
			return fmt.Errorf("nil chunk received")
		}

		written, err := tmpfile.Write(ck.Content)
		if err != nil {
			klog.Errorf("error writing to temp file: %s", err)
			return fmt.Errorf("error writing to temp file: %w", err)
		}
		fsize += int64(written)
	}

	klog.Infof("tag file received, size: %d bytes", fsize)
	if _, err := tmpfile.Seek(0, 0); err != nil {
		klog.Errorf("error file seek: %s", err)
		return fmt.Errorf("error file seek: %w", err)
	}

	if err := t.tagexp.Import(ctx, namespace, name, tmpfile); err != nil {
		klog.Errorf("error importing tag: %s", err)
		return fmt.Errorf("error importing tag: %w", err)
	}

	klog.Infof("tag %s/%s imported successfully", namespace, name)
	return stream.SendAndClose(&pb.ImportResult{})
}

// Name returns a name identifier for this controller.
func (t *TagIO) Name() string {
	return "tag input/output handler"
}

// Start puts the http server online. TODO enable ssl on this listener.
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
