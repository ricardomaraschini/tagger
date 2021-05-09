package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Tag is a map between an internal kubernetes image tag and a remote image
// hosted in a remote registry
type Tag struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status TagStatus `json:"status,omitempty"`
	Spec   TagSpec   `json:"spec,omitempty"`
}

// CurrentReferenceForTag looks through provided tag and returns the ref
// in use. Image tag generation in status points to the current generation,
// if this generation does not exist then we haven't imported it yet,
// return an empty string instead.
func (t *Tag) CurrentReferenceForTag() string {
	for _, hashref := range t.Status.References {
		if hashref.Generation != t.Status.Generation {
			continue
		}
		return hashref.ImageReference
	}
	return ""
}

// NextGeneration returns what the next generation for a given tag should
// be. It is the last imported generation plus one. Returns zero if Tag
// hasn't been imported yet.
func (t *Tag) NextGeneration() int64 {
	var next int64
	if len(t.Status.References) > 0 {
		next = t.Status.References[0].Generation + 1
	}
	return next
}

// SpecTagImported returs true if tag generation defined on spec has
// already been imported (exists in status.References).
func (t *Tag) SpecTagImported() bool {
	for _, hashref := range t.Status.References {
		if hashref.Generation != t.Spec.Generation {
			continue
		}
		return true
	}
	return false
}

// ValidateTagGeneration checks if tag's spec information is valid. Generation
// may be set to any already imported generation or to a new one (last imported
// generation + 1).
func (t *Tag) ValidateTagGeneration() error {
	validGens := []int64{0}
	if len(t.Status.References) > 0 {
		validGens = []int64{t.Status.References[0].Generation + 1}
		for _, ref := range t.Status.References {
			validGens = append(validGens, ref.Generation)
		}
	}
	for _, gen := range validGens {
		if gen != t.Spec.Generation {
			continue
		}
		return nil
	}
	return fmt.Errorf("generation must be one of: %s", fmt.Sprint(validGens))
}

// PrependHashReference prepends ref into tag's HashReference slice. The resulting
// slice contains at most 10 references.
func (t *Tag) PrependHashReference(ref HashReference) {
	newRefs := []HashReference{ref}
	newRefs = append(newRefs, t.Status.References...)
	// TODO make this value (10) configurable.
	if len(newRefs) > 10 {
		newRefs = newRefs[:10]
	}
	t.Status.References = newRefs
}

// RegisterImportFailure updates the last import attempt struct in Tag status, setting
// it as not succeeded and with the proper error message.
func (t *Tag) RegisterImportFailure(err error) {
	t.Status.LastImportAttempt = ImportAttempt{
		When:    metav1.Now(),
		Succeed: false,
		Reason:  err.Error(),
	}
}

// RegisterImportSuccess updates the last import attempt struct in Tag status, setting
// it as succeeded.
func (t *Tag) RegisterImportSuccess() {
	t.Status.LastImportAttempt = ImportAttempt{
		When:    metav1.Now(),
		Succeed: true,
	}
}

// TagSpec represents the user intention with regards to tagging
// remote images.
type TagSpec struct {
	From       string `json:"from"`
	Mirror     bool   `json:"mirror"`
	Generation int64  `json:"generation"`
}

// TagStatus is the current status for an image tag.
type TagStatus struct {
	Generation        int64           `json:"generation"`
	References        []HashReference `json:"references"`
	LastImportAttempt ImportAttempt   `json:"lastImportAttempt"`
}

// ImportAttempt holds data about an import cycle. Keeps track if it
// was successful, when it happens and if not successful what was the
// error reported (on reason).
type ImportAttempt struct {
	When    metav1.Time `json:"when"`
	Succeed bool        `json:"succeed"`
	Reason  string      `json:"reason,omitempty"`
}

// HashReference is an reference to a image hash in a given generation.
type HashReference struct {
	Generation     int64       `json:"generation"`
	From           string      `json:"from"`
	ImportedAt     metav1.Time `json:"importedAt"`
	ImageReference string      `json:"imageReference,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TagList is a list of Tag.
type TagList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []Tag `json:"items"`
}
