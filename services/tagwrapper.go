package services

import (
	"fmt"

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
// already been imported.
func (w TagWrapper) SpecTagImported() bool {
	for _, hashref := range w.Status.References {
		if hashref.Generation == w.Spec.Generation {
			return true
		}
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
