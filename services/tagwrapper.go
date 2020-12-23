package services

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

// TagWrapper extends an object of type pointer to a Tag with extra methods.
type TagWrapper struct {
	*imagtagv1.Tag
}

// CurrentReferenceForTag looks through provided tag and returns the ref
// in use. Image tag generation in status points to the current generation,
// if this generation does not exist then we haven't imported it yet,
// return an empty string instead.
func (w TagWrapper) CurrentReferenceForTag() string {
	for _, hashref := range w.Status.References {
		if hashref.Generation != w.Status.Generation {
			continue
		}
		return hashref.ImageReference
	}
	return ""
}

// SpecTagImported returs true if tag generation defined on spec has
// already been imported (exists in status.References).
func (w TagWrapper) SpecTagImported() bool {
	for _, hashref := range w.Status.References {
		if hashref.Generation != w.Spec.Generation {
			continue
		}
		return true
	}
	return false
}

// ValidateTagGeneration checks if tag's spec information is valid. Generation
// may be set to any already imported generation or to a new one (last imported
// generation + 1).
func (w TagWrapper) ValidateTagGeneration() error {
	validGens := []int64{0}
	if len(w.Status.References) > 0 {
		validGens = []int64{w.Status.References[0].Generation + 1}
		for _, ref := range w.Status.References {
			validGens = append(validGens, ref.Generation)
		}
	}
	for _, gen := range validGens {
		if gen != w.Spec.Generation {
			continue
		}
		return nil
	}
	return fmt.Errorf("generation must be one of: %s", fmt.Sprint(validGens))
}

// PrependHashReference prepends ref into tag's HashReference slice. The resulting
// slice contains at most 5 references.
func (w TagWrapper) PrependHashReference(ref imagtagv1.HashReference) {
	newRefs := []imagtagv1.HashReference{ref}
	newRefs = append(newRefs, w.Status.References...)
	// TODO make this value (5) configurable.
	if len(newRefs) > 5 {
		newRefs = newRefs[:5]
	}
	w.Status.References = newRefs
}

// RegisterImportFailure updates the last import attempt struct in Tag status, setting
// it as not succeeded and with the proper error message.
func (w TagWrapper) RegisterImportFailure(err error) {
	w.Status.LastImportAttempt = imagtagv1.ImportAttempt{
		When:    metav1.Now(),
		Succeed: false,
		Reason:  err.Error(),
	}
}

// RegisterImportSuccess updates the last import attempt struct in Tag status, setting
// it as succeeded.
func (w TagWrapper) RegisterImportSuccess() {
	w.Status.LastImportAttempt = imagtagv1.ImportAttempt{
		When:    metav1.Now(),
		Succeed: true,
	}
}
