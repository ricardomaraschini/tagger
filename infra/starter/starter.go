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

package starter

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	corecli "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/google/uuid"
)

// Controller is implemented by all controllers inside controllers directory.  They should be
// able to be started, have a name, and to inform if they need or not to be run only after a
// leader election.
type Controller interface {
	Start(ctx context.Context) error
	RequiresLeaderElection() bool
	Name() string
}

// Starter provides facilities around starting Controllers. Leader election logic is embed into
// this entity.
type Starter struct {
	corcli    corecli.Interface
	name      string
	namespace string
	wg        sync.WaitGroup
	ctrls     []Controller
	cancel    context.CancelFunc
}

// New returns a new controller starter. We read some env variables directly here and fall back
// to default values if they are not set.
func New(corcli corecli.Interface, ctrls ...Controller) *Starter {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "tagger"
		klog.Warning("unbound POD_NAMESPACE, using 'tagger'")
	}

	name := os.Getenv("POD_NAME")
	if name == "" {
		name = uuid.New().String()
		klog.Warningf("unbound POD_NAME, using %s", name)
	}

	return &Starter{
		corcli:    corcli,
		namespace: namespace,
		name:      name,
		ctrls:     ctrls,
	}
}

// OnStartedLeading is called when we start being the leader of the pack.  Goes through all
// controllers and start the ones that require a leader lease in place to run. Each controller
// is started in its own goroutine.
func (s *Starter) OnStartedLeading(ctx context.Context) {
	klog.Infof("we are now the leader, starting controllers.")
	for _, c := range s.ctrls {
		if !c.RequiresLeaderElection() {
			continue
		}
		s.wg.Add(1)
		go s.startController(ctx, c)
	}
}

// OnStoppedLeading awaits for all running controllers to end before returning. As we don't know
// exactly why we are not the leader anymore we just cancel our internal context and wait for all
// the Controllers to finish before returning.
func (s *Starter) OnStoppedLeading() {
	klog.Infof("we are no longer the leader, ending controllers.")
	s.cancel()
	s.wg.Wait()
}

// startController calls Start() in a Controller.
func (s *Starter) startController(ctx context.Context, c Controller) {
	defer s.wg.Done()
	klog.Infof("starting controller %q.", c.Name())
	if err := c.Start(ctx); err != nil {
		klog.Errorf("controller %q failed: %s", c.Name(), err)
		return
	}
	klog.Infof("%q controller ended.", c.Name())
}

// Start starts all controllers within a Starter. This function only returns when all controllers
// have finished their job, i.e. provided context has been cancelled or the leader lease has been
// lost. lockID holds an arbitrary ID for the binary calling this function, it is used as config
// map name during leader election.
func (s *Starter) Start(ctx context.Context, lockID string) error {
	// we wrap the provided context into our own context. This way we can cancel everything
	// here if we feel like doing so. All controllers receive this context as theirs during
	// a Start() call.
	ctx, s.cancel = context.WithCancel(ctx)

	lock, err := resourcelock.New(
		resourcelock.ConfigMapsResourceLock,
		s.namespace,
		lockID,
		s.corcli.CoreV1(),
		s.corcli.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: s.name,
		},
	)
	if err != nil {
		return fmt.Errorf("error creating resource lock: %w", err)
	}

	election, err := leaderelection.NewLeaderElector(
		leaderelection.LeaderElectionConfig{
			Lock:            lock,
			LeaseDuration:   time.Minute,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			ReleaseOnCancel: false,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: s.OnStartedLeading,
				OnStoppedLeading: s.OnStoppedLeading,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error creating elector: %w", err)
	}

	// let's start the controllers that do not require leader election right here.
	// Controllers requiring leader election are going to be started as soon as we
	// get a lease, see OnStartedLeading.
	for _, c := range s.ctrls {
		if c.RequiresLeaderElection() {
			continue
		}
		s.wg.Add(1)
		go s.startController(ctx, c)
	}

	// runs the leader election. this locks the flow here until the context is cancelled
	// or we lost the leadership.
	election.Run(ctx)
	return nil
}
