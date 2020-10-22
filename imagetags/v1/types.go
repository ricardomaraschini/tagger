package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	From               string `json:"from"`
	Generations        int64  `json:"generations"`
	DeployedGeneration int64  `json:"deployedGeneration"`
}

// TagStatus is the current status for an image tag.
type TagStatus struct {
	DeployedGeneration int64           `json:"deployedGeneration"`
	References         []HashReference `json:"references"`
}

// HashReference is an reference to a image hash in a given generation.
type HashReference struct {
	Generation     int64       `json:"generation"`
	From           string      `json:"from"`
	LastUpdatedAt  metav1.Time `json:"lastUpdatedAt"`
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
