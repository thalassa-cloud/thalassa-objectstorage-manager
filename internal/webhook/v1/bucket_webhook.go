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
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

var bucketlog = logf.Log.WithName("bucket-resource")

const deleteProtectionDeniedMsg = "deleteProtection is enabled; set spec.deleteProtection=false before deleting"

// SetupBucketWebhookWithManager registers the webhook for Bucket in the manager.
func SetupBucketWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &objectstoragev1.Bucket{}).
		WithValidator(&BucketCustomValidator{}).
		Complete()
}

// NOTE: verbs=delete only — create/update stay available if the webhook is briefly unreachable.
// +kubebuilder:webhook:path=/validate-objectstorage-controllers-thalassa-cloud-v1-bucket,mutating=false,failurePolicy=fail,sideEffects=None,groups=objectstorage.controllers.thalassa.cloud,resources=buckets,verbs=delete,versions=v1,name=vbucket-v1.kb.io,admissionReviewVersions=v1

// BucketCustomValidator validates Bucket admission requests.
//
// +kubebuilder:object:generate=false
type BucketCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator.
func (v *BucketCustomValidator) ValidateCreate(_ context.Context, obj *objectstoragev1.Bucket) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *BucketCustomValidator) ValidateUpdate(_ context.Context, _, _ *objectstoragev1.Bucket) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator.
func (v *BucketCustomValidator) ValidateDelete(_ context.Context, obj *objectstoragev1.Bucket) (admission.Warnings, error) {
	if obj == nil {
		return nil, nil
	}
	bucketlog.Info("validating Bucket deletion", "name", obj.GetName(), "namespace", obj.GetNamespace())
	if obj.Spec.DeleteProtection {
		return nil, fmt.Errorf("%s", deleteProtectionDeniedMsg)
	}
	return nil, nil
}
