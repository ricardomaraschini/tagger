package controllers

import (
	"context"
	"time"

	"github.com/ricardomaraschini/tagger/services"
	"k8s.io/apimachinery/pkg/api/errors"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

// Boot is a controller to make sure the environment contains all
// needed objects so this operator can work properly.
type Boot struct {
	sctlister corelister.SecretLister
	sctsvc    *services.Secret
}

// NewBoot returns a new bootstrap controller.
func NewBoot(sctlister corelister.SecretLister, sctsvc *services.Secret) *Boot {
	return &Boot{
		sctlister: sctlister,
		sctsvc:    sctsvc,
	}
}

// Start initiates the sync loop for the bootstrap objects. I have chosen to
// just use a loop to conciliate everything, no need for watchers.
func (b *Boot) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		sct, err := b.sctlister.Secrets("tagger").Get("certs")
		if err != nil {
			if !errors.IsNotFound(err) {
				klog.Errorf("error reading secret: %s", err)
				continue
			}

			sct, err = b.sctsvc.CreateCertificates(ctx)
			if err != nil {
				klog.Errorf("error creating secret: %s", err)
				continue
			}
		}

		written, err := b.sctsvc.CopySecret(sct)
		if err != nil {
			klog.Errorf("error copying secret: %s", err)
			continue
		}

		if !written {
			continue
		}
	}
}
