/*
Copyright 2026 Thalassa Cloud.

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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BucketVersioning controls object versioning on the bucket.
// +kubebuilder:validation:Enum=Disabled;Enabled;Suspended
type BucketVersioning string

const (
	BucketVersioningDisabled  BucketVersioning = "Disabled"
	BucketVersioningEnabled   BucketVersioning = "Enabled"
	BucketVersioningSuspended BucketVersioning = "Suspended"
)

// BucketSpec defines the desired state of a Thalassa object storage bucket.
type BucketSpec struct {
	// Metadata allows optional name and label overrides for the resource in Thalassa.
	// When metadata.name is set it is used as the bucket name (immutable after create).
	// +optional
	Metadata *ResourceMetadata `json:"metadata,omitempty"`

	// Name is the base Thalassa bucket name. If empty, metadata.name or the Kubernetes object name is used.
	// When generateNameSuffix is enabled (the default), a short random suffix is appended on create
	// (e.g. "app-data" → "app-data-a1b2c3"). The final name is recorded in status.bucketName.
	// Immutable after the bucket is created.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name,omitempty"`

	// GenerateNameSuffix appends a short random suffix to the bucket name on create so repeated
	// applies get unique Thalassa bucket names. Defaults to true. Set to false for a fixed name
	// (required for deterministic naming / adoption by exact name).
	// +optional
	// +kubebuilder:default=true
	GenerateNameSuffix *bool `json:"generateNameSuffix,omitempty"`

	// Region is the Thalassa cloud region identity or slug (immutable after create).
	// If empty, the manager uses --default-region.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="region is immutable"
	Region string `json:"region,omitempty"`

	// Public controls whether the bucket allows public access.
	// +optional
	Public bool `json:"public,omitempty"`

	// Versioning is the bucket versioning mode.
	// +optional
	// +kubebuilder:default=Disabled
	Versioning BucketVersioning `json:"versioning,omitempty"`

	// ObjectLockEnabled enables object lock on the bucket.
	// +optional
	ObjectLockEnabled bool `json:"objectLockEnabled,omitempty"`

	// Policy is an optional S3-compatible bucket policy as a JSON string.
	// BucketAccess-managed statements use a reserved Sid prefix and are merged by the BucketAccess controller.
	// +optional
	Policy string `json:"policy,omitempty"`
}

// BucketUsage reports observed storage usage for the bucket.
type BucketUsage struct {
	// TotalSize is the total size of objects in the bucket.
	// +optional
	TotalSize resource.Quantity `json:"totalSize,omitempty"`

	// TotalObjects is the number of objects in the bucket.
	// +optional
	TotalObjects int64 `json:"totalObjects,omitempty"`
}

// BucketStatus defines the observed state of Bucket.
type BucketStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the bucket.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// BucketName is the Thalassa bucket name used for API calls.
	// +optional
	BucketName string `json:"bucketName,omitempty"`

	// Endpoint is the S3-compatible endpoint URL for the bucket.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Region is the resolved Thalassa region slug or identity.
	// +optional
	Region string `json:"region,omitempty"`

	// Usage is the observed usage of the bucket.
	// +optional
	Usage BucketUsage `json:"usage,omitempty"`

	// Conditions represent the current state of the Bucket.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.status.bucketName`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.region`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Bucket is the Schema for Thalassa object storage buckets.
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec"`
	Status BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket.
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Bucket{}, &BucketList{})
}
