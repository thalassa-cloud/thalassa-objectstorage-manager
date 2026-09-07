/*
Copyright 2026 Thalassa Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    10|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrincipalKind is the Thalassa IAM principal kind for external grants.
// +kubebuilder:validation:Enum=User;ServiceAccount
type PrincipalKind string

const (
	PrincipalKindUser           PrincipalKind = "User"
	PrincipalKindServiceAccount PrincipalKind = "ServiceAccount"
)

// PrincipalReference points at an existing Thalassa IAM principal (user or service account).
// When set on BucketAccess, the controller only manages bucket policy grants for that principal
// and does not create credentials or write a Secret.
type PrincipalReference struct {
	// Kind is the Thalassa IAM principal kind.
	// +kubebuilder:validation:Required
	Kind PrincipalKind `json:"kind"`

	// Identity is the Thalassa identity of the principal (e.g. u-... for users, sa-... for service accounts).
	// For users this is typically the AppUser subject.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Identity string `json:"identity"`
}

// BucketAccessSpec defines the desired state of BucketAccess.
//
// Two modes:
//   - Managed (default): create a Thalassa IAM service account + object-storage credentials,
//     grant that principal on referenced buckets, and write S3 credentials into a Secret.
//     Requires writeSecretToRef; principalRef must be unset.
//   - External principal: grant an existing User or ServiceAccount on referenced buckets only.
//     Requires principalRef; writeSecretToRef must be unset. No credentials are created or deleted.
//
// +kubebuilder:validation:XValidation:rule="has(self.principalRef) != has(self.writeSecretToRef)",message="exactly one of principalRef or writeSecretToRef must be set"
type BucketAccessSpec struct {
	// BucketRefs references one or more Bucket resources in the same namespace.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	BucketRefs []LocalObjectReference `json:"bucketRefs"`

	// WriteSecretToRef is the Secret that will receive access credentials (managed mode).
	// Keys written: accessKey, secretKey, endpoint, bucket (primary), buckets (comma-separated).
	// Required when principalRef is unset; must be omitted when principalRef is set.
	// +optional
	WriteSecretToRef *SecretReference `json:"writeSecretToRef,omitempty"`

	// PrincipalRef references an existing Thalassa IAM User or ServiceAccount (external mode).
	// When set, the controller only upserts a managed bucket policy statement for that principal.
	// Required when writeSecretToRef is unset; must be omitted in managed mode.
	// +optional
	PrincipalRef *PrincipalReference `json:"principalRef,omitempty"`

	// ServiceAccountName overrides the Thalassa IAM service account name (managed mode only).
	// Defaults to "<namespace>-<name>" of this BucketAccess. Ignored when principalRef is set.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Description is an optional description for the Thalassa IAM service account (managed mode only).
	// +optional
	Description string `json:"description,omitempty"`

	// PermissionPreset selects a built-in S3 action set when Permissions is empty.
	// ReadOnly = get/list objects; ReadWrite = default object read/write + list (recommended default).
	// Ignored when Permissions is non-empty.
	// +optional
	// +kubebuilder:validation:Enum=ReadOnly;ReadWrite
	// +kubebuilder:default=ReadWrite
	PermissionPreset string `json:"permissionPreset,omitempty"`

	// Permissions is an explicit list of S3 actions for the managed bucket policy statement.
	// Each action must be allowlisted by the controller (admin actions like s3:*, DeleteBucket,
	// PutBucketPolicy are rejected). When empty, PermissionPreset is used.
	// +optional
	Permissions []string `json:"permissions,omitempty"`

	// ExpiresAt is an optional expiration timestamp for the access credential (managed mode only).
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// Metadata allows optional labels/annotations on the Thalassa IAM service account (managed mode only).
	// +optional
	Metadata *ResourceMetadata `json:"metadata,omitempty"`
}

// BucketAccessStatus defines the observed state of BucketAccess.
type BucketAccessStatus struct {
	ReconcileStatus `json:",inline"`

	// ServiceAccountID is the Thalassa identity of the IAM service account
	// (managed SA, or the referenced ServiceAccount in external mode).
	// +optional
	ServiceAccountID string `json:"serviceAccountId,omitempty"`

	// PrincipalARN is the Thalassa IAM principal ARN last applied to bucket policies.
	// +optional
	PrincipalARN string `json:"principalArn,omitempty"`

	// AccessCredentialID is the Thalassa identity of the access credential (managed mode only).
	// +optional
	AccessCredentialID string `json:"accessCredentialId,omitempty"`

	// AccessKeyID is the non-secret access key identifier (never the secret; managed mode only).
	// +optional
	AccessKeyID string `json:"accessKeyId,omitempty"`

	// AppliedPermissions is the resolved S3 action list last applied to bucket policies.
	// +optional
	AppliedPermissions []string `json:"appliedPermissions,omitempty"`

	// LastCredentialRotationTime is when credentials were last created or rotated (managed mode only).
	// +optional
	LastCredentialRotationTime *metav1.Time `json:"lastCredentialRotationTime,omitempty"`

	// GrantedBuckets lists Thalassa bucket names that currently have the managed policy grant.
	// +optional
	GrantedBuckets []string `json:"grantedBuckets,omitempty"`

	// SecretRef names the Secret that holds credentials (managed mode only).
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Conditions represent the current state of the BucketAccess.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Principal",type=string,JSONPath=`.status.principalArn`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.secretRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BucketAccess provisions Thalassa IAM credentials and/or bucket policy grants for S3 access.
type BucketAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketAccessSpec   `json:"spec"`
	Status BucketAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketAccessList contains a list of BucketAccess.
type BucketAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BucketAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BucketAccess{}, &BucketAccessList{})
}
