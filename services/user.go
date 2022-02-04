// Copyright 2020 The Imageger Authors.
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

	authev1 "k8s.io/api/authentication/v1"
	authov1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corecli "k8s.io/client-go/kubernetes"
)

// User entity gather operations related to Kubernetes users such
// as token validations and authorization.
type User struct {
	corcli corecli.Interface
}

// NewUser returns an User handler capable of managing authentication and
// authorization for Kubernetes users.
func NewUser(corcli corecli.Interface) *User {
	return &User{
		corcli: corcli,
	}
}

// CanUpdateImages returns nil if provided token is able to update Image entities
// in a namespace.
func (u *User) CanUpdateImages(ctx context.Context, ns, token string) error {
	if _, err := u.corcli.CoreV1().Namespaces().Get(
		ctx, ns, metav1.GetOptions{},
	); err != nil {
		return err
	}

	tkreview := &authev1.TokenReview{
		Spec: authev1.TokenReviewSpec{
			Token: token,
		},
	}

	tr, err := u.corcli.AuthenticationV1().TokenReviews().Create(
		ctx, tkreview, metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}
	if !tr.Status.Authenticated {
		return fmt.Errorf("user not authenticated")
	}

	subreview := &authov1.SubjectAccessReview{
		Spec: authov1.SubjectAccessReviewSpec{
			User:   tr.Status.User.Username,
			Groups: tr.Status.User.Groups,
			ResourceAttributes: &authov1.ResourceAttributes{
				Namespace: ns,
				Resource:  "images",
				Verb:      "update",
				Group:     "tagger.dev",
			},
		},
	}

	autho, err := u.corcli.AuthorizationV1().SubjectAccessReviews().Create(
		ctx, subreview, metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}

	if !autho.Status.Allowed || autho.Status.Denied {
		return fmt.Errorf("unauthorized access")
	}
	return nil
}
