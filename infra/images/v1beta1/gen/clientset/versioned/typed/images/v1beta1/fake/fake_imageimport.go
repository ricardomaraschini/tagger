/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1beta1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeImageImports implements ImageImportInterface
type FakeImageImports struct {
	Fake *FakeTaggerV1beta1
	ns   string
}

var imageimportsResource = schema.GroupVersionResource{Group: "tagger.dev", Version: "v1beta1", Resource: "imageimports"}

var imageimportsKind = schema.GroupVersionKind{Group: "tagger.dev", Version: "v1beta1", Kind: "ImageImport"}

// Get takes name of the imageImport, and returns the corresponding imageImport object, and an error if there is any.
func (c *FakeImageImports) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta1.ImageImport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(imageimportsResource, c.ns, name), &v1beta1.ImageImport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.ImageImport), err
}

// List takes label and field selectors, and returns the list of ImageImports that match those selectors.
func (c *FakeImageImports) List(ctx context.Context, opts v1.ListOptions) (result *v1beta1.ImageImportList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(imageimportsResource, imageimportsKind, c.ns, opts), &v1beta1.ImageImportList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.ImageImportList{ListMeta: obj.(*v1beta1.ImageImportList).ListMeta}
	for _, item := range obj.(*v1beta1.ImageImportList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested imageImports.
func (c *FakeImageImports) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(imageimportsResource, c.ns, opts))

}

// Create takes the representation of a imageImport and creates it.  Returns the server's representation of the imageImport, and an error, if there is any.
func (c *FakeImageImports) Create(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.CreateOptions) (result *v1beta1.ImageImport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(imageimportsResource, c.ns, imageImport), &v1beta1.ImageImport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.ImageImport), err
}

// Update takes the representation of a imageImport and updates it. Returns the server's representation of the imageImport, and an error, if there is any.
func (c *FakeImageImports) Update(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (result *v1beta1.ImageImport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(imageimportsResource, c.ns, imageImport), &v1beta1.ImageImport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.ImageImport), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeImageImports) UpdateStatus(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (*v1beta1.ImageImport, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(imageimportsResource, "status", c.ns, imageImport), &v1beta1.ImageImport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.ImageImport), err
}

// Delete takes name of the imageImport and deletes it. Returns an error if one occurs.
func (c *FakeImageImports) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(imageimportsResource, c.ns, name), &v1beta1.ImageImport{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeImageImports) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(imageimportsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta1.ImageImportList{})
	return err
}

// Patch applies the patch and returns the patched imageImport.
func (c *FakeImageImports) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.ImageImport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(imageimportsResource, c.ns, name, pt, data, subresources...), &v1beta1.ImageImport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.ImageImport), err
}