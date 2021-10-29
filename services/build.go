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

package services

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	shpv1alpha1 "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	shpwinf "github.com/shipwright-io/build/pkg/client/informers/externalversions"
	shpwlist "github.com/shipwright-io/build/pkg/client/listers/build/v1alpha1"

	tagclient "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	taginform "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/informers/externalversions"
)

// Build is a struct that gathers all actions related to shipwright's BuildRun objects.
type Build struct {
	brinf  shpwinf.SharedInformerFactory
	brlis  shpwlist.BuildRunLister
	tagsvc *Tag
	syssvc *SysContext
}

// NewBuild returns a new service entity providing services (actions) for shipwright buildrun
// objects. This struct uses Tag service struct to deal with Tag objects.
func NewBuild(
	corinf informers.SharedInformerFactory,
	tagcli tagclient.Interface,
	taginf taginform.SharedInformerFactory,
	brinf shpwinf.SharedInformerFactory,
) *Build {
	var brlis shpwlist.BuildRunLister
	if brinf != nil {
		brlis = brinf.Shipwright().V1alpha1().BuildRuns().Lister()
	}

	return &Build{
		brinf:  brinf,
		brlis:  brlis,
		tagsvc: NewTag(corinf, tagcli, taginf),
		syssvc: NewSysContext(corinf),
	}
}

// hasSucceeded returns if provided BuildRun has been succeeded. This is useful
// when determining if a BuildRun can be turned into a Tag (or a new generation
// of an existing Tag). We can only create a Tag (or a new generation for an
// existing one) if the BuildRun has succeeded and the image is already hosted
// in a remote registry.
func (b *Build) hasSucceeded(br *shpv1alpha1.BuildRun) bool {
	for _, cond := range br.Status.Conditions {
		if cond.Type != shpv1alpha1.Succeeded {
			continue
		}
		return cond.Status == corev1.ConditionTrue
	}
	return false
}

// validateBuildRun validates if a BuildRun contain all fields we need in order
// to sync it with a Tag. This does not mean that the BuildRun is invalid but
// that we can't yet turn it into a Tag (i.e. it is invalid for this operator).
func (b *Build) validateBuildRun(br *shpv1alpha1.BuildRun) error {
	if br.Spec.BuildRef == nil {
		return fmt.Errorf("build run without a build reference")
	}
	if br.Status.BuildSpec == nil {
		return fmt.Errorf("unable to find build spec in status")
	}
	if br.Status.CompletionTime == nil {
		return fmt.Errorf("build run does not contain competion time")
	}
	return nil
}

// Sync syncs a build run. The goal is to have a Tag with the BuildName referenced
// by the BuildRun. If no Tag exists one is created, if Tag exists then a new
// generation for the Tag is created.
func (b *Build) Sync(ctx context.Context, br *shpv1alpha1.BuildRun) error {
	if !b.hasSucceeded(br) {
		klog.Infof("build %s/%s run not succeeded", br.Namespace, br.Name)
		return nil
	}

	if err := b.validateBuildRun(br); err != nil {
		return fmt.Errorf("%s/%s: %w", br.Namespace, br.Name, err)
	}
	tagname := br.Spec.BuildRef.Name
	imgdest := br.Status.BuildSpec.Output.Image

	tag, err := b.tagsvc.Get(ctx, br.Namespace, br.Spec.BuildRef.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("unable to get tag: %w", err)
		}

		// attempts to load the backend registry, this function will succeed if
		// there is a valid configuration for it. If it fails then we can't use
		// mirror.
		usemirror := true
		if _, err := b.syssvc.GetRegistryStore(ctx); err != nil {
			klog.Infof("won't be able to mirror build run image: %s", err)
			usemirror = false
		}
		return b.tagsvc.NewTag(ctx, br.Namespace, tagname, imgdest, usemirror)
	}

	// if there is an inflight import for the Tag postpone the sync.
	if !tag.SpecTagImported() {
		klog.Infof("build tag still importing")
		return nil
	}

	// XXX we need a better way of determining if a new generation for a Tag
	// must be created. For now we are only checking if the Tag last import
	// is younger than the build run completion time.
	lastimport := tag.Status.LastImportAttempt.When
	buildfinish := br.Status.CompletionTime
	if lastimport.After(buildfinish.Time) {
		klog.Infof("tag already imported after build competion")
		return nil
	}

	// bump the Tag generation, this will make the tag to be reimported.
	// XXX this is wrong as the BuildRun's output image may change, we still
	// need to cover this.
	if _, err := b.tagsvc.NewGeneration(ctx, br.Namespace, tagname); err != nil {
		return fmt.Errorf("error upgrading tag for build run: %w", err)
	}
	return nil
}

// AddEventHandler adds a handler to BuildRuns related events.
func (b *Build) AddEventHandler(handler cache.ResourceEventHandler) {
	b.brinf.Shipwright().V1alpha1().BuildRuns().Informer().AddEventHandler(handler)
}

// Get returns a BuildRun object. Returned object is already a copy of the cached
// object and may be modified by caller as needed.
func (b *Build) Get(ctx context.Context, ns, name string) (*shpv1alpha1.BuildRun, error) {
	br, err := b.brlis.BuildRuns(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("unable to get build run: %w", err)
	}
	return br.DeepCopy(), nil
}
