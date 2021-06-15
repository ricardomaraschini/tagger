// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/ricardomaraschini/tagger/infra/progbar"
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
// You might be looking for a concrete implementation of this, please
// look at services/user.go and you will find it.
type UserValidator interface {
	CanUpdateTags(context.Context, string, string) error
}

// TagIO handles requests for pulling and pushing current image pointed
// by a Tag.
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
// this). The implementation here is made so we have a stateless handler.
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
func (t *TagIO) Pull(in *pb.Packet, stream pb.TagIOService_PullServer) error {
	ctx := stream.Context()
	head := in.GetHeader()
	if err := t.authorizeRequest(ctx, head); err != nil {
		klog.Errorf("error validating pull request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	fp, cleanup, err := t.tagexp.Pull(ctx, head.GetNamespace(), head.GetName())
	if err != nil {
		klog.Errorf("error pulling tag image: %s", err)
		return fmt.Errorf("error pulling tag image: %w", err)
	}
	defer cleanup()

	finfo, err := fp.Stat()
	if err != nil {
		klog.Errorf("error calculating total image size: %s", err)
		return fmt.Errorf("error calculating total image size: %s", err)
	}
	fsize := finfo.Size()

	return pb.Send(fp, fsize, stream, progbar.NewNoOp())
}

// Push handles image pushes through grpc. The first message received indicates
// the image destination (tag's namespace and name) and a authorization token,
// all subsequent messages are of type Chunk where we can find a slice of bytes.
// We reassemble the image on disk and later on Load it into a registry.
func (t *TagIO) Push(stream pb.TagIOService_PushServer) error {
	ctx := stream.Context()
	in, err := stream.Recv()
	if err != nil {
		klog.Errorf("error receiving import request: %s", err)
		return fmt.Errorf("error receiving import request: %w", err)
	}

	head := in.GetHeader()
	if err := t.authorizeRequest(ctx, head); err != nil {
		klog.Errorf("error validating export request: %s", err)
		return fmt.Errorf("error validating input: %w", err)
	}

	tmpfile, cleanup, err := t.fs.TempFile()
	if err != nil {
		klog.Errorf("error creating temp file: %s", err)
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer cleanup()

	if err := pb.Receive(stream, tmpfile, progbar.NewNoOp()); err != nil {
		klog.Errorf("error receiving image through grpc: %s", err)
		return fmt.Errorf("error receiving image through grpc: %w", err)
	}

	// Push now pushes the local image file into mirror registry.
	if err := t.tagexp.Push(
		ctx, head.GetNamespace(), head.GetName(), tmpfile.Name(),
	); err != nil {
		klog.Errorf("error importing tag: %s", err)
		return fmt.Errorf("error importing tag: %w", err)
	}
	return stream.SendAndClose(&pb.Packet{})
}

// authorizeRequest checks if all mandatory fields in a request are present.
// It also does the validation if the token is capable of acessing tags in
// provided namespace.
func (t *TagIO) authorizeRequest(ctx context.Context, head *pb.Header) error {
	if head == nil {
		return fmt.Errorf("nil protobuf request")
	}
	if head.GetName() == "" {
		return fmt.Errorf("empty name field")
	}
	if head.GetNamespace() == "" {
		return fmt.Errorf("empty namespace field")
	}
	if head.GetToken() == "" {
		return fmt.Errorf("empty token field")
	}
	return t.usrval.CanUpdateTags(
		ctx, head.GetNamespace(), head.GetToken(),
	)
}

// Name returns a name identifier for this controller.
func (t *TagIO) Name() string {
	return "tag images io handler"
}

// RequiresLeaderElection returns if this controller requires or not a
// leader lease to run. On our case, as we are a grpc server, we do not
// require to be a leader in order to work properly.
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
