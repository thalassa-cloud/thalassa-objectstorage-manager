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

package bucketaccess

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/iam"
	"github.com/thalassa-cloud/client-go/objectstorage"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/conditions"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/secretref"
	helpers "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/helpers"
)

const (
	Finalizer = "objectstorage.controllers.thalassa.cloud/bucketaccess"

	requeueAfterStatusUpdateFailure = 15 * time.Second

	secretKeyAccessKey = "accessKey"
	secretKeySecretKey = "secretKey"
	secretKeyEndpoint  = "endpoint"
	secretKeyBucket    = "bucket"
	secretKeyBuckets   = "buckets"

	ownershipLabelKey = "objectstorage.controllers.thalassa.cloud/owned-by"
)

// Config holds dependencies for Handler.
type Config struct {
	Client                      client.Client
	Scheme                      *runtime.Scheme
	IAM                         *iam.Client
	ObjectStorage               *objectstorage.Client
	OrganisationID              string
	AllowAllNamespacesSecretRef bool
	Recorder                    record.EventRecorder
}

// Handler implements BucketAccess lifecycle.
type Handler struct {
	Client                      client.Client
	Scheme                      *runtime.Scheme
	IAM                         *iam.Client
	ObjectStorage               *objectstorage.Client
	OrganisationID              string
	AllowAllNamespacesSecretRef bool
	Recorder                    record.EventRecorder
}

func NewHandler(cfg Config) *Handler {
	return &Handler{
		Client:                      cfg.Client,
		Scheme:                      cfg.Scheme,
		IAM:                         cfg.IAM,
		ObjectStorage:               cfg.ObjectStorage,
		OrganisationID:              cfg.OrganisationID,
		AllowAllNamespacesSecretRef: cfg.AllowAllNamespacesSecretRef,
		Recorder:                    cfg.Recorder,
	}
}

type resolvedBucket struct {
	Name     string
	Endpoint string
}

func (h *Handler) Reconcile(ctx context.Context, obj *objectstoragev1.BucketAccess) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if strings.TrimSpace(h.OrganisationID) == "" {
		return h.setErrorCondition(ctx, obj, "OrganisationRequired",
			"organisation ID is required for bucket policy principal ARNs; set --organisation",
			fmt.Errorf("organisation ID is not configured"))
	}

	buckets, err := h.resolveBuckets(ctx, obj)
	if err != nil {
		if err == helpers.ErrDependencyNotReady {
			stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "WaitingForBucket", "Referenced Bucket is not ready yet")
			_ = h.updateStatusWithRetry(ctx, obj)
			return ctrl.Result{RequeueAfter: helpers.RequeueAfterDependencyNotReady}, nil
		}
		return h.setErrorCondition(ctx, obj, "BucketRefFailed", err.Error(), err)
	}

	managed := IsManagedMode(obj)
	if !managed && WantsCredentialRotation(obj.GetAnnotations()) {
		return h.setErrorCondition(ctx, obj, "RotationNotSupported",
			"credential rotation is only supported in managed mode (omit principalRef)",
			fmt.Errorf("rotate-credentials annotation is not valid with principalRef"))
	}

	progressMsg := "Ensuring IAM service account and credentials"
	if !managed {
		progressMsg = "Granting bucket access to external principal"
	}
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Reconciling", progressMsg)
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	sid := ManagedSid(obj.Namespace, obj.Name)
	actions, err := ResolvePermissions(obj.Spec.PermissionPreset, obj.Spec.Permissions)
	if err != nil {
		return h.setErrorCondition(ctx, obj, "InvalidPermissions", err.Error(), err)
	}
	obj.Status.AppliedPermissions = actions

	var (
		principalARN string
		saID         string
		accessKey    string
		accessSecret string
		credID       string
		rotated      bool
	)

	if managed {
		if obj.Spec.WriteSecretToRef == nil || obj.Spec.WriteSecretToRef.Name == "" {
			return h.setErrorCondition(ctx, obj, "SecretRefRequired",
				"writeSecretToRef is required in managed mode",
				fmt.Errorf("writeSecretToRef is required when principalRef is unset"))
		}

		saID, err = h.ensureServiceAccount(ctx, obj)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "ServiceAccountFailed", err.Error(), err)
		}
		obj.Status.ServiceAccountID = saID

		accessKey, accessSecret, credID, rotated, err = h.ensureAccessCredential(ctx, obj, saID)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "AccessCredentialFailed", err.Error(), err)
		}
		obj.Status.AccessCredentialID = credID
		obj.Status.AccessKeyID = accessKey
		principalARN = ServiceAccountPrincipalARN(h.OrganisationID, saID)
	} else {
		resolved, err := h.resolveExternalPrincipal(ctx, obj.Spec.PrincipalRef)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "PrincipalRefFailed", err.Error(), err)
		}
		principalARN = resolved.ARN
		saID = resolved.ServiceAccountID
		obj.Status.ServiceAccountID = saID
		// Clear managed-only status fields in external mode.
		obj.Status.AccessCredentialID = ""
		obj.Status.AccessKeyID = ""
		obj.Status.SecretRef = nil
		obj.Status.LastCredentialRotationTime = nil
	}
	obj.Status.PrincipalARN = principalARN

	granted := make([]string, 0, len(buckets))
	for _, b := range buckets {
		if err := h.ensureBucketGrant(ctx, b.Name, sid, principalARN, actions); err != nil {
			return h.setErrorCondition(ctx, obj, "PolicyGrantFailed", err.Error(), err)
		}
		granted = append(granted, b.Name)
	}
	obj.Status.GrantedBuckets = granted

	if managed {
		secretNS, err := secretref.Resolve(obj.Namespace, obj.Spec.WriteSecretToRef.Namespace, h.AllowAllNamespacesSecretRef)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "SecretRefInvalid", err.Error(), err)
		}
		if err := h.writeSecret(ctx, obj, secretNS, accessKey, accessSecret, buckets); err != nil {
			return h.setErrorCondition(ctx, obj, "SecretWriteFailed", err.Error(), err)
		}
		obj.Status.SecretRef = &objectstoragev1.SecretReference{Name: obj.Spec.WriteSecretToRef.Name, Namespace: secretNS}

		if rotated {
			if err := h.clearRotateAnnotation(ctx, obj); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
			}
			now := metav1.Now()
			obj.Status.LastCredentialRotationTime = &now
			h.Recorder.Eventf(obj, corev1.EventTypeNormal, "CredentialsRotated", "Rotated object storage access credentials")
		} else if obj.Status.LastCredentialRotationTime == nil {
			now := metav1.Now()
			obj.Status.LastCredentialRotationTime = &now
		}
	}

	obj.Status.LastReconcileError = ""
	obj.Status.ResourceStatus = helpers.ResourceStatusReady
	readyMsg := "Bucket access credentials are ready"
	if !managed {
		readyMsg = "External principal bucket grant is ready"
	}
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateAvailable, "Ready", readyMsg)
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	log.Info("reconciled BucketAccess",
		"managed", managed,
		"principalArn", principalARN,
		"serviceAccountId", saID,
		"credentialId", credID,
		"rotated", rotated,
	)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (h *Handler) resolveBuckets(ctx context.Context, obj *objectstoragev1.BucketAccess) ([]resolvedBucket, error) {
	out := make([]resolvedBucket, 0, len(obj.Spec.BucketRefs))
	for _, ref := range obj.Spec.BucketRefs {
		if ref.Name == "" {
			return nil, fmt.Errorf("bucketRefs entry has empty name")
		}
		var bucket objectstoragev1.Bucket
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: ref.Name}, &bucket); err != nil {
			return nil, fmt.Errorf("get Bucket %s/%s: %w", obj.Namespace, ref.Name, err)
		}
		if bucket.Status.BucketName == "" || bucket.Status.ResourceID == "" {
			return nil, helpers.ErrDependencyNotReady
		}
		ready := false
		for _, c := range bucket.Status.Conditions {
			if c.Type == stdconditions.ConditionTypeReady && c.Status == metav1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return nil, helpers.ErrDependencyNotReady
		}
		out = append(out, resolvedBucket{Name: bucket.Status.BucketName, Endpoint: bucket.Status.Endpoint})
	}
	return out, nil
}

func (h *Handler) ensureServiceAccount(ctx context.Context, obj *objectstoragev1.BucketAccess) (string, error) {
	if obj.Status.ServiceAccountID != "" {
		sa, err := h.IAM.GetServiceAccount(ctx, obj.Status.ServiceAccountID)
		if err == nil && sa != nil {
			return sa.Identity, nil
		}
		if err != nil && !thalassaclient.IsNotFound(err) {
			return "", err
		}
	}

	name := obj.Spec.ServiceAccountName
	if name == "" {
		name = fmt.Sprintf("%s-%s", obj.Namespace, obj.Name)
	}
	var desc *string
	if obj.Spec.Description != "" {
		d := obj.Spec.Description
		desc = &d
	}
	labels := helpers.EffectiveLabels(obj.Spec.Metadata)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[ownershipLabelKey] = fmt.Sprintf("%s.%s", obj.Namespace, obj.Name)

	created, err := h.IAM.CreateServiceAccount(ctx, iam.CreateServiceAccountRequest{
		Name:        name,
		Description: desc,
		Labels:      labels,
		Annotations: helpers.EffectiveAnnotations(obj.Spec.Metadata),
	})
	if err != nil {
		return "", err
	}
	h.Recorder.Eventf(obj, corev1.EventTypeNormal, "ServiceAccountCreated", "Created Thalassa IAM service account %s", created.Identity)
	return created.Identity, nil
}

func (h *Handler) ensureAccessCredential(ctx context.Context, obj *objectstoragev1.BucketAccess, saID string) (accessKey, accessSecret, credID string, rotated bool, err error) {
	if obj.Spec.WriteSecretToRef == nil {
		return "", "", "", false, fmt.Errorf("writeSecretToRef is required")
	}
	secretNS, err := secretref.Resolve(obj.Namespace, obj.Spec.WriteSecretToRef.Namespace, h.AllowAllNamespacesSecretRef)
	if err != nil {
		return "", "", "", false, err
	}

	rotate := WantsCredentialRotation(obj.GetAnnotations())
	existingKey, existingSecret, secretOK := h.readSecretCredentials(ctx, secretNS, obj.Spec.WriteSecretToRef.Name)

	// Reuse existing credential material unless rotation was requested.
	if !rotate && secretOK {
		if obj.Status.AccessCredentialID != "" {
			return existingKey, existingSecret, obj.Status.AccessCredentialID, false, nil
		}
		creds, listErr := h.IAM.GetServiceAccountAccessCredentials(ctx, saID)
		if listErr == nil {
			for _, c := range creds {
				if c.AccessKey == existingKey {
					return existingKey, existingSecret, c.Identity, false, nil
				}
			}
		}
	}

	oldCredID := obj.Status.AccessCredentialID

	var expires *time.Time
	if obj.Spec.ExpiresAt != nil {
		t := obj.Spec.ExpiresAt.Time
		expires = &t
	}
	req := iam.CreateServiceAccountAccessCredentialRequest{
		Name:      fmt.Sprintf("%s-%s", obj.Namespace, obj.Name),
		ExpiresAt: expires,
		Scopes:    []iam.AccessCredentialsScope{iam.AccessCredentialsScopeObjectStorage},
	}
	created, err := h.IAM.CreateServiceAccountAccessCredentials(ctx, saID, req)
	if err != nil {
		return "", "", "", false, err
	}

	// Delete previous credential after successful create (rotation / replacement).
	if oldCredID != "" && oldCredID != created.Identity {
		if delErr := h.IAM.DeleteServiceAccountAccessCredentials(ctx, saID, oldCredID); delErr != nil && !thalassaclient.IsNotFound(delErr) {
			logf.FromContext(ctx).Error(delErr, "failed to delete previous access credential after rotation", "oldCredentialId", oldCredID)
		}
	}

	h.Recorder.Eventf(obj, corev1.EventTypeNormal, "AccessCredentialCreated", "Created object storage access credential %s", created.Identity)
	return created.AccessKey, created.AccessSecret, created.Identity, rotate || oldCredID != "", nil
}

func (h *Handler) clearRotateAnnotation(ctx context.Context, obj *objectstoragev1.BucketAccess) error {
	if !WantsCredentialRotation(obj.GetAnnotations()) {
		return nil
	}
	return retry.OnError(retry.DefaultRetry, func(err error) bool { return true }, func() error {
		var latest objectstoragev1.BucketAccess
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(obj), &latest); err != nil {
			return err
		}
		if !WantsCredentialRotation(latest.GetAnnotations()) {
			return nil
		}
		anns := latest.GetAnnotations()
		delete(anns, RotateCredentialsAnnotation)
		latest.SetAnnotations(anns)
		if err := h.Client.Update(ctx, &latest); err != nil {
			return err
		}
		obj.SetAnnotations(latest.GetAnnotations())
		return nil
	})
}

func (h *Handler) readSecretCredentials(ctx context.Context, namespace, name string) (accessKey, accessSecret string, ok bool) {
	var secret corev1.Secret
	if err := h.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		return "", "", false
	}
	ak := string(secret.Data[secretKeyAccessKey])
	sk := string(secret.Data[secretKeySecretKey])
	if ak == "" || sk == "" {
		return "", "", false
	}
	return ak, sk, true
}

func (h *Handler) ensureBucketGrant(ctx context.Context, bucketName, sid, principalARN string, actions []string) error {
	bucket, err := h.ObjectStorage.GetBucket(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("get bucket %s: %w", bucketName, err)
	}
	policy := UpsertManagedStatement(bucket.Policy, sid, principalARN, bucketName, actions)
	objectLock := bucket.ObjectLockEnabled
	_, err = h.ObjectStorage.UpdateBucket(ctx, bucketName, objectstorage.UpdateBucketRequest{
		Public:            bucket.Public,
		PolicyDocument:    &policy,
		Versioning:        bucket.Versioning,
		ObjectLockEnabled: &objectLock,
		Labels:            bucket.Labels,
		Annotations:       bucket.Annotations,
	})
	return err
}

func (h *Handler) writeSecret(ctx context.Context, obj *objectstoragev1.BucketAccess, secretNS, accessKey, accessSecret string, buckets []resolvedBucket) error {
	if obj.Spec.WriteSecretToRef == nil {
		return fmt.Errorf("writeSecretToRef is required")
	}
	if accessKey == "" || accessSecret == "" {
		return fmt.Errorf("missing credential material for secret write")
	}
	names := make([]string, 0, len(buckets))
	endpoint := ""
	for i, b := range buckets {
		names = append(names, b.Name)
		if i == 0 {
			endpoint = b.Endpoint
		}
	}

	var existing corev1.Secret
	err := h.Client.Get(ctx, types.NamespacedName{Namespace: secretNS, Name: obj.Spec.WriteSecretToRef.Name}, &existing)
	switch {
	case err == nil:
		if !secretOwnedBy(obj, &existing) {
			return fmt.Errorf("secret %s/%s exists and is not owned by BucketAccess %s/%s; refusing to overwrite",
				secretNS, obj.Spec.WriteSecretToRef.Name, obj.Namespace, obj.Name)
		}
	case apierrors.IsNotFound(err):
		// create path below
	default:
		return fmt.Errorf("get secret %s/%s: %w", secretNS, obj.Spec.WriteSecretToRef.Name, err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Spec.WriteSecretToRef.Name,
			Namespace: secretNS,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, h.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Type = corev1.SecretTypeOpaque
		secret.Data[secretKeyAccessKey] = []byte(accessKey)
		secret.Data[secretKeySecretKey] = []byte(accessSecret)
		secret.Data[secretKeyEndpoint] = []byte(endpoint)
		if len(names) > 0 {
			secret.Data[secretKeyBucket] = []byte(names[0])
			secret.Data[secretKeyBuckets] = []byte(strings.Join(names, ","))
		}
		// Only set owner reference when secret is in the same namespace (K8s restriction).
		if secretNS == obj.Namespace {
			return controllerutil.SetControllerReference(obj, secret, h.Scheme)
		}
		// Cross-namespace: stamp a label for ownership tracking (no ownerRef across namespaces).
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[ownershipLabelKey] = fmt.Sprintf("%s.%s", obj.Namespace, obj.Name)
		return nil
	})
	return err
}

func secretOwnedBy(obj *objectstoragev1.BucketAccess, secret *corev1.Secret) bool {
	for _, ref := range secret.GetOwnerReferences() {
		if ref.UID == obj.UID && ref.Kind == "BucketAccess" && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	// Cross-namespace secrets use an ownership label instead of ownerRef.
	if secret.Labels != nil {
		want := fmt.Sprintf("%s.%s", obj.Namespace, obj.Name)
		if secret.Labels[ownershipLabelKey] == want {
			return true
		}
	}
	return false
}

func (h *Handler) Terminate(ctx context.Context, obj *objectstoragev1.BucketAccess) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(obj, Finalizer) {
		return ctrl.Result{}, nil
	}

	sid := ManagedSid(obj.Namespace, obj.Name)
	for _, name := range obj.Status.GrantedBuckets {
		if err := h.removeBucketGrant(ctx, name, sid); err != nil {
			log.Error(err, "failed to remove managed bucket policy statement", "bucket", name)
			return ctrl.Result{}, err
		}
	}
	// Also try currently referenced buckets in case status was incomplete.
	for _, ref := range obj.Spec.BucketRefs {
		var bucket objectstoragev1.Bucket
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: ref.Name}, &bucket); err != nil {
			continue
		}
		if bucket.Status.BucketName != "" {
			_ = h.removeBucketGrant(ctx, bucket.Status.BucketName, sid)
		}
	}

	// Only delete IAM credentials / service account in managed mode.
	// External principals are never deleted by this controller.
	if IsManagedMode(obj) {
		if obj.Status.ServiceAccountID != "" && obj.Status.AccessCredentialID != "" {
			if err := h.IAM.DeleteServiceAccountAccessCredentials(ctx, obj.Status.ServiceAccountID, obj.Status.AccessCredentialID); err != nil && !thalassaclient.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		if obj.Status.ServiceAccountID != "" {
			if err := h.IAM.DeleteServiceAccount(ctx, obj.Status.ServiceAccountID); err != nil && !thalassaclient.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			h.Recorder.Eventf(obj, corev1.EventTypeNormal, "ServiceAccountDeleted", "Deleted Thalassa IAM service account %s", obj.Status.ServiceAccountID)
		}
	}

	if controllerutil.RemoveFinalizer(obj, Finalizer) {
		if err := h.Client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (h *Handler) removeBucketGrant(ctx context.Context, bucketName, sid string) error {
	bucket, err := h.ObjectStorage.GetBucket(ctx, bucketName)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			return nil
		}
		return err
	}
	policy := RemoveManagedStatement(bucket.Policy, sid)
	objectLock := bucket.ObjectLockEnabled
	_, err = h.ObjectStorage.UpdateBucket(ctx, bucketName, objectstorage.UpdateBucketRequest{
		Public:            bucket.Public,
		PolicyDocument:    &policy,
		Versioning:        bucket.Versioning,
		ObjectLockEnabled: &objectLock,
		Labels:            bucket.Labels,
		Annotations:       bucket.Annotations,
	})
	return err
}

func (h *Handler) updateStatusWithRetry(ctx context.Context, obj *objectstoragev1.BucketAccess) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest objectstoragev1.BucketAccess
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(obj), &latest); err != nil {
			return err
		}
		latest.Status = obj.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setErrorCondition(ctx context.Context, obj *objectstoragev1.BucketAccess, reason, message string, err error) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	obj.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, obj); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	if apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: helpers.RequeueAfterDependencyNotReady}, nil
	}
	// Return the error alone; controller-runtime ignores RequeueAfter when err != nil.
	return ctrl.Result{}, err
}
