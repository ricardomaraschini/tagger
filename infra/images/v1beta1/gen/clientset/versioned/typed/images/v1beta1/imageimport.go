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

package v1beta1

import (
	"context"
	"time"

	v1beta1 "github.com/ricardomaraschini/tagger/infra/images/v1beta1"
	scheme "github.com/ricardomaraschini/tagger/infra/images/v1beta1/gen/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ImageImportsGetter has a method to return a ImageImportInterface.
// A group's client should implement this interface.
type ImageImportsGetter interface {
	ImageImports(namespace string) ImageImportInterface
}

// ImageImportInterface has methods to work with ImageImport resources.
type ImageImportInterface interface {
	Create(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.CreateOptions) (*v1beta1.ImageImport, error)
	Update(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (*v1beta1.ImageImport, error)
	UpdateStatus(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (*v1beta1.ImageImport, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1beta1.ImageImport, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1beta1.ImageImportList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.ImageImport, err error)
	ImageImportExpansion
}

// imageImports implements ImageImportInterface
type imageImports struct {
	client rest.Interface
	ns     string
}

// newImageImports returns a ImageImports
func newImageImports(c *TaggerV1beta1Client, namespace string) *imageImports {
	return &imageImports{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the imageImport, and returns the corresponding imageImport object, and an error if there is any.
func (c *imageImports) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta1.ImageImport, err error) {
	result = &v1beta1.ImageImport{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("imageimports").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ImageImports that match those selectors.
func (c *imageImports) List(ctx context.Context, opts v1.ListOptions) (result *v1beta1.ImageImportList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1beta1.ImageImportList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("imageimports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested imageImports.
func (c *imageImports) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("imageimports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a imageImport and creates it.  Returns the server's representation of the imageImport, and an error, if there is any.
func (c *imageImports) Create(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.CreateOptions) (result *v1beta1.ImageImport, err error) {
	result = &v1beta1.ImageImport{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("imageimports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(imageImport).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a imageImport and updates it. Returns the server's representation of the imageImport, and an error, if there is any.
func (c *imageImports) Update(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (result *v1beta1.ImageImport, err error) {
	result = &v1beta1.ImageImport{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("imageimports").
		Name(imageImport.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(imageImport).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *imageImports) UpdateStatus(ctx context.Context, imageImport *v1beta1.ImageImport, opts v1.UpdateOptions) (result *v1beta1.ImageImport, err error) {
	result = &v1beta1.ImageImport{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("imageimports").
		Name(imageImport.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(imageImport).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the imageImport and deletes it. Returns an error if one occurs.
func (c *imageImports) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("imageimports").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *imageImports) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("imageimports").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched imageImport.
func (c *imageImports) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.ImageImport, err error) {
	result = &v1beta1.ImageImport{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("imageimports").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
