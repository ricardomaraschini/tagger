package controllers

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"k8s.io/klog/v2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/ricardomaraschini/tagger/infra/fs"
	"github.com/ricardomaraschini/tagger/infra/pb"
)

// ImagePusherPuller is here to make tests easier. You may be looking
// for its concrete implementation in services/tagio.go. The goal of
// an ImagePusherPuller is to allow us to Push and Pull images to and
// from our mirror registry.
type ImagePusherPuller interface {
	Push(context.Context, string, string, string) error
	Pull(context.Context, string, string) (*os.File, func(), error)
}

// UserValidator validates an user can access Tags in a given namespace.
// You should be looking for a concrete implementation of this, please
// look at services/user.go and you will find it.
type UserValidator interface {
	CanAccessTags(context.Context, string, string) error
}

// TagIO handles requests for pulling and pushing images pointed by a
// Tag.
type TagIO struct {
	bind   string
	tagexp ImagePusherPuller
	usrval UserValidator
	srv    *grpc.Server
	fs     *fs.FS
	pb.UnimplementedTagIOServiceServer
}

// NewTagIO returns a grpc handler for image Pull and Push requests. I
// have hardcoded what seems to be reasonable values in terms of keep
// alive and connection lifespan management (we may need to better tune
// this).
func NewTagIO(tagexp ImagePusherPuller, usrval UserValidator) *TagIO {
	aliveopt := grpc.KeepaliveParams(
		keepalive.ServerParameters{
			MaxConnectionIdle:     time.Minute,
			MaxConnectionAge:      20 * time.Minute,
			MaxConnectionAgeGrace: time.Minute,
			Time:                  time.Second,
			Timeout:               5 * time.Second,
		},
	)
	tio := &TagIO{
		bind:   ":8083",
		tagexp: tagexp,
		usrval: usrval,
		fs:     fs.New(""),
		srv:    grpc.NewServer(aliveopt),
	}
	pb.RegisterTagIOServiceServer(tio.srv, tio)
	reflection.Register(tio.srv)
	return tio
}

// Pull handles an image pull through grpc. We receive a request informing what
// is the Tag to be pulled from (namespace and name) and also a kubernetes token
// for authentication and authorization.
func (t *TagIO) Pull(in *pb.Request, stream pb.TagIOService_PullServer) error {
	ctx := stream.Context()
	if err := t.authorizeRequest(ctx, in); err != nil {
		klog.Errorf("error validating pull request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	fp, cleanup, err := t.tagexp.Pull(ctx, in.GetNamespace(), in.GetName())
	if err != nil {
		klog.Errorf("error pulling tag image: %s", err)
		return fmt.Errorf("error pulling tag image: %w", err)
	}
	defer cleanup()

	return pb.SendFileServer(fp, stream)
}

// Push handles image pushes through grpc. The first message received indicates
// the image destination (tag's namespace and name) and a authorization token,
// all subsequent messages are of type Chunk where we can find a slice of bytes.
func (t *TagIO) Push(stream pb.TagIOService_PushServer) error {
	ctx := stream.Context()
	in, err := stream.Recv()
	if err != nil {
		klog.Errorf("error receiving import request: %s", err)
		return fmt.Errorf("error receiving import request: %w", err)
	}

	req := in.GetRequest()
	if err := t.authorizeRequest(ctx, req); err != nil {
		klog.Errorf("error validating export request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	tmpfile, cleanup, err := t.fs.TempFile()
	if err != nil {
		klog.Errorf("error creating temp file: %s", err)
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer cleanup()

	written, err := pb.ReceiveFileServer(tmpfile, stream)
	if err != nil {
		klog.Errorf("error receiving image through grpc: %s", err)
		return fmt.Errorf("error receiving image through grpc: %w", err)
	}
	klog.Infof("tag image received, size: %d bytes", written)

	// Push now pushes the local image file into mirror registry.
	if err := t.tagexp.Push(
		ctx, req.GetNamespace(), req.GetName(), tmpfile.Name(),
	); err != nil {
		klog.Errorf("error importing tag: %s", err)
		return fmt.Errorf("error importing tag: %w", err)
	}
	return stream.SendAndClose(&pb.PushResult{})
}

// authorizeRequest checks if all mandatory fields in a request are present.
// It also does the validation if the token is capable of acessing tags in
// provided namespace.
func (t *TagIO) authorizeRequest(ctx context.Context, req *pb.Request) error {
	if req == nil {
		return fmt.Errorf("nil protobuf request")
	}
	if req.GetName() == "" {
		return fmt.Errorf("empty name field")
	}
	if req.GetNamespace() == "" {
		return fmt.Errorf("empty namespace field")
	}
	if req.GetToken() == "" {
		return fmt.Errorf("empty token field")
	}
	return t.usrval.CanAccessTags(
		ctx, req.GetNamespace(), req.GetToken(),
	)
}

// Name returns a name identifier for this controller.
func (t *TagIO) Name() string {
	return "tag images io handler"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run.
func (t *TagIO) RequiresLeaderElection() bool {
	return false
}

// Start puts the grpc server online. TODO enable ssl on this listener.
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
