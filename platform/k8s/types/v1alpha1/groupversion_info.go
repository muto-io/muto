// +groupName=muto.io
// +kubebuilder:object:generate=true

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	//nolint:staticcheck // SA1019: scheme.Builder is deprecated but required for proper type registration
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "muto.io", Version: "v1alpha1"}
	//nolint:staticcheck // SA1019: scheme.Builder is deprecated but required for proper type registration
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
