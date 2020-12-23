package v1

import (
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

// TagSpec represents the user intention with regards to tagging
// remote images.
type TagSpec struct {
	From       string `json:"from"`
	Cache      bool   `json:"cache"`
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
