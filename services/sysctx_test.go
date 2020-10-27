package services

import (
	"context"

	"github.com/containers/image/v5/types"
)

type SysContextMock struct {
	unqualifiedRegistries []string
}

func (s *SysContextMock) UnqualifiedRegistries(imgPath context.Context) []string {
	return s.unqualifiedRegistries
}

func (s *SysContextMock) AuthsFor(
	context.Context, types.ImageReference, string,
) ([]*types.DockerAuthConfig, error) {
	return nil, nil
}
