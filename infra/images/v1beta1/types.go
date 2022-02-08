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

package v1beta1

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var (
	// MaxImportAttempts holds how many times we gonna try to import an ImageImport object
	// before giving up.
	MaxImportAttempts = 10
	// MaxImageHReferences tells us how many image references a Image can hold on its status.
	MaxImageHReferences = 25
	// GroupVersion is a string that holds "group/version" for the resources of this package.
	GroupVersion = fmt.Sprintf("%s/%s", SchemeGroupVersion.Group, SchemeGroupVersion.Version)
	// ImageKind holds the kind we use when saving Images in the k8s API.
	ImageKind = "Image"
	// ImageImportKind holds the kind we use when saving ImageImports in the k8s API.
	ImageImportKind = "ImageImport"
	// ImageImportConsumedFlagAnnotation is the annotation set in an ImageImport object
	// whenever the temporary ImageImport object has already been consumed and is not
	// needed anymore.
	ImageImportConsumedFlagAnnotation = "tagger.dev/consumed"
	// ConditionTypeImported is a condition we report in ImageImport objects, presenting the
	// current Import status back to the user.
	ConditionTypeImported = "Imported"
	// ConditionReasonProgressing is used to indicate the ImageImport is progressing.
	ConditionReasonProgressing = "Progressing"
	// ConditionReasonImageImported is used to indicate an ImageImport attempt has been
	// executed sucessfully.
	ConditionReasonImageImported = "ImageImported"
	// ConditionReasonNoMoreAttempts is used when we can't proceed attempting to process an
	// ImageImport object.
	ConditionReasonNoMoreAttempts = "NoMoreAttempts"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Image is a map between an internal kubernetes image tag and multiple remote hosted images.
type Image struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageSpec   `json:"spec,omitempty"`
	Status ImageStatus `json:"status,omitempty"`
}

// PrependFinishedImports calls PrependFinishedImport for each ImageImport in the slice.
func (t *Image) PrependFinishedImports(imps []ImageImport) {
	for _, imp := range imps {
		t.PrependFinishedImport(imp)
	}
}

// PrependFinishedImport prepends provided ImageImport into Image status hash references,
// keeps MaxImageHReferences references. We do not prepend the provided ImageImport if the
// most recent import in the Image points to the same image.
func (t *Image) PrependFinishedImport(imp ImageImport) {
	if imp.Status.HashReference == nil {
		return
	}
	href := *imp.Status.HashReference

	// we do not prepend if the most recent import has the same image reference.  in this
	// scenario we only update From and ImportedAt to reflect this newly triggered import.
	if len(t.Status.HashReferences) > 0 {
		lref := t.Status.HashReferences[0]
		if href.ImageReference == lref.ImageReference {
			lref.From = href.From
			lref.ImportedAt = href.ImportedAt
			t.Status.HashReferences[0] = lref
			return
		}
	}

	all := append([]HashReference{href}, t.Status.HashReferences...)
	if len(all) > MaxImageHReferences {
		all = all[0:MaxImageHReferences]
	}

	t.Status.HashReferences = all
}

// Validate checks Image contain all mandatory fields.
func (t *Image) Validate() error {
	if t.Spec.From == "" {
		return fmt.Errorf("empty spec.from")
	}
	return nil
}

// FlagAsConsumed is used whenever we have already processed the data in an ImageImport object.
// This Annotation does not indicate anything at the k8s scope and it is solely used inside this
// operator. The value in the annotation is the current date and time encoded as time.ANSIC.
func (t *ImageImport) FlagAsConsumed() {
	if t.Annotations == nil {
		t.Annotations = map[string]string{}
	}
	t.Annotations[ImageImportConsumedFlagAnnotation] = time.Now().Format(time.ANSIC)
}

// FlaggedAsConsumed returns if this ImageImport is flagged for deletion. Inspects the
// object's Annotations.
func (t *ImageImport) FlaggedAsConsumed() bool {
	_, ok := t.Annotations[ImageImportConsumedFlagAnnotation]
	return ok
}

// FlaggedAsConsumedDuration returns the amount of time that has passed since the ImageImport
// was flagged for deletion.
func (t *ImageImport) FlaggedAsConsumedDuration() (time.Duration, error) {
	if !t.FlaggedAsConsumed() {
		return 0, fmt.Errorf("image import not flagged for deletion")
	}

	strsince := t.Annotations[ImageImportConsumedFlagAnnotation]
	since, err := time.Parse(time.ANSIC, strsince)
	if err != nil {
		return 0, fmt.Errorf(
			"bogus %s annotation: %w", ImageImportConsumedFlagAnnotation, err,
		)
	}

	return time.Now().Sub(since), nil
}

// CurrentReferenceForImage looks through provided Image  and returns the most recent imported
// reference found (first item in .status.HashReferences).
func (t *Image) CurrentReferenceForImage() string {
	if len(t.Status.HashReferences) == 0 {
		return ""
	}
	return t.Status.HashReferences[0].ImageReference
}

// ImageSpec represents the user intention with regards to importing remote images.
type ImageSpec struct {
	From     string `json:"from"`
	Mirror   bool   `json:"mirror"`
	Insecure bool   `json:"insecure"`
}

// ImageStatus is the current status for an Image.
type ImageStatus struct {
	HashReferences []HashReference `json:"hashReferences,omitempty"`
}

// ImportAttempt holds data about an import cycle. Keeps track if it was successful, when it
// happened and if not successful what was the error reported (reason).
type ImportAttempt struct {
	When    metav1.Time `json:"when"`
	Succeed bool        `json:"succeed"`
	Reason  string      `json:"reason,omitempty"`
}

// HashReference is an reference to an imported Image (by its sha).
type HashReference struct {
	From           string      `json:"from"`
	ImportedAt     metav1.Time `json:"importedAt"`
	ImageReference string      `json:"imageReference,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageList is a list of Image.
type ImageList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Image `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageImport represents a request, made by the user, to import a Image from a remote repository
// and into an Image object.
type ImageImport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageImportSpec   `json:"spec,omitempty"`
	Status ImageImportStatus `json:"status,omitempty"`
}

// OwnedByImage returns true if ImageImport is owned by provided Image.
func (t *ImageImport) OwnedByImage(img *Image) bool {
	orefs := t.GetOwnerReferences()
	for _, oref := range orefs {
		if oref.Kind != ImageKind {
			continue
		}
		if oref.APIVersion != GroupVersion {
			continue
		}
		if oref.Name != img.Name {
			continue
		}
		if oref.UID != img.UID {
			continue
		}
		return true
	}
	return false
}

// SetOwnerImage makes sure that provided ImageImport has provided Image among its owners.
func (t *ImageImport) SetOwnerImage(img *Image) {
	orefs := append(
		t.GetOwnerReferences(),
		metav1.OwnerReference{
			Kind:       ImageKind,
			APIVersion: GroupVersion,
			Name:       img.Name,
			UID:        img.UID,
		},
	)
	t.SetOwnerReferences(orefs)
}

// Validate checks ImageImport contain all mandatory fields.
func (t *ImageImport) Validate() error {
	if t.Spec.TargetImage == "" {
		return fmt.Errorf("empty spec.targetImage")
	}
	return nil
}

// InheritValuesFrom uses provided Image to set default values for required propertis in a
// ImageImport before processing it. For example if no "From" has been specified in the
// ImageImport object we read it from the provided Image object. This function guarantees
// that there will be no nil pointers in the ImageImport spec property.
func (t *ImageImport) InheritValuesFrom(it *Image) {
	if t.Spec.TargetImage == "" {
		t.Spec.TargetImage = it.Name
	}

	if t.Spec.From == "" {
		t.Spec.From = it.Spec.From
	}

	if t.Spec.Insecure == nil {
		t.Spec.Insecure = pointer.Bool(it.Spec.Insecure)
	}

	if t.Spec.Mirror == nil {
		t.Spec.Mirror = pointer.Bool(it.Spec.Mirror)
	}
}

// AlreadyImported checks if a given ImageImport has already been executed, we evaluate this by
// inspecting if we already have a HashReference for the image in its Status.
func (t *ImageImport) AlreadyImported() bool {
	return t.Status.HashReference != nil
}

// FailedImportAttempts returns the number of failed import attempts.
func (t *ImageImport) FailedImportAttempts() int {
	count := 0
	for _, att := range t.Status.ImportAttempts {
		if !att.Succeed {
			count++
		}
	}
	return count
}

// RegisterImportFailure updates the import attempts slice appending a new failed attempt with
// the provided error. This function also sets ImageImport.Status.Condition field.
func (t *ImageImport) RegisterImportFailure(err error) {
	t.Status.ImportAttempts = append(
		t.Status.ImportAttempts,
		ImportAttempt{
			When:    metav1.Now(),
			Succeed: false,
			Reason:  err.Error(),
		},
	)

	// we build kind of a base Condition and then adjust only the necessary fields. This
	// base Condition means that we have failed all attempts at processing an ImportImage.
	message := fmt.Sprintf("Import attempt %d/%d", MaxImportAttempts, MaxImportAttempts)
	nextcond := metav1.Condition{
		Type:               ConditionTypeImported,
		Status:             metav1.ConditionFalse,
		Reason:             ConditionReasonNoMoreAttempts,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}

	failures := len(t.Status.ImportAttempts)
	if failures >= MaxImportAttempts {
		// here we have exhausted all import attempts, set it as Failed and return.
		t.Status.Condition = nextcond
		return
	}

	// if we hit here then we still have some import attempts to be executed, set its
	// condition to Progressing.
	message = fmt.Sprintf("Import attempt %d/%d", failures, MaxImportAttempts)
	nextcond.Status = metav1.ConditionFalse
	nextcond.Reason = ConditionReasonProgressing
	nextcond.Message = message
	nextcond.LastTransitionTime = t.Status.Condition.LastTransitionTime
	if nextcond.LastTransitionTime.IsZero() {
		nextcond.LastTransitionTime = metav1.NewTime(time.Now())
	}
	t.Status.Condition = nextcond
}

// RegisterImportSuccess appends a new ImportAttempt to the status registering it worked as
// expected. This function also sets ImageImport.Status.Condition field.
func (t *ImageImport) RegisterImportSuccess() {
	t.Status.ImportAttempts = append(
		t.Status.ImportAttempts,
		ImportAttempt{
			When:    metav1.Now(),
			Succeed: true,
		},
	)

	t.Status.Condition = metav1.Condition{
		Type:               ConditionTypeImported,
		Status:             metav1.ConditionTrue,
		Reason:             ConditionReasonImageImported,
		Message:            "Image imported successfully",
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

// ImageImportSpec represents the body of the request to import a given container image tag from
// a remote location. Values not set in here are read from the TargetImage, e.g.  if no "mirror"
// is set here but it is set in the targetImage we use it.
type ImageImportSpec struct {
	TargetImage string `json:"targetImage"`
	From        string `json:"from"`
	Mirror      *bool  `json:"mirror,omitempty"`
	Insecure    *bool  `json:"insecure,omitempty"`
}

// ImageImportStatus holds the current status for an image tag import attempt.
type ImageImportStatus struct {
	Condition      metav1.Condition `json:"condition"`
	ImportAttempts []ImportAttempt  `json:"importAttempts"`
	HashReference  *HashReference   `json:"hashReference,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageImportList is a list of ImageImport objects.
type ImageImportList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ImageImport `json:"items"`
}
