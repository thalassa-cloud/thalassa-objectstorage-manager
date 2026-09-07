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

package bucket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/objectstorage"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/conditions"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/bucketaccess"
	helpers "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/helpers"
)

const (
	Finalizer                       = "objectstorage.controllers.thalassa.cloud/bucket"
	requeueAfterStatusUpdateFailure = 15 * time.Second
	// maxBucketNameLen is the S3-compatible bucket name length limit.
	maxBucketNameLen = 63
	// nameSuffixBytes is the entropy used for the random name suffix (hex-encoded → 2*n chars).
	nameSuffixBytes = 3
	// maxCreateNameAttempts retries create with a new suffix if the generated name collides.
	maxCreateNameAttempts = 5
)

// Config holds dependencies for Handler.
type Config struct {
	Client                 client.Client
	Scheme                 *runtime.Scheme
	ObjectStorage          *objectstorage.Client
	DefaultRegion          string
	BucketAdoptionEnabled  bool
	AdoptionRequiredLabels map[string]string
	Recorder               record.EventRecorder
}

// Handler implements Bucket lifecycle against the Thalassa object storage API.
type Handler struct {
	Client                 client.Client
	Scheme                 *runtime.Scheme
	ObjectStorage          *objectstorage.Client
	DefaultRegion          string
	BucketAdoptionEnabled  bool
	AdoptionRequiredLabels map[string]string
	Recorder               record.EventRecorder
}

func NewHandler(cfg Config) *Handler {
	labels := cfg.AdoptionRequiredLabels
	if len(labels) == 0 {
		labels = DefaultAdoptionRequiredLabels()
	}
	return &Handler{
		Client:                 cfg.Client,
		Scheme:                 cfg.Scheme,
		ObjectStorage:          cfg.ObjectStorage,
		DefaultRegion:          cfg.DefaultRegion,
		BucketAdoptionEnabled:  cfg.BucketAdoptionEnabled,
		AdoptionRequiredLabels: labels,
		Recorder:               cfg.Recorder,
	}
}

func (h *Handler) Reconcile(ctx context.Context, obj *objectstoragev1.Bucket) (ctrl.Result, error) {
	if obj.Status.BucketName == "" && obj.Status.ResourceID == "" {
		return h.createBucket(ctx, obj)
	}
	name, ok := provisionedBucketName(obj)
	if !ok {
		return h.setErrorCondition(ctx, obj, "BucketNameUnknown",
			"status.bucketName is empty while generateNameSuffix is enabled; refusing to use the base name (it may identify a different bucket)",
			fmt.Errorf("status.bucketName is required when generateNameSuffix is enabled"))
	}
	return h.reconcileBucket(ctx, obj, name)
}

// EffectiveBucketName returns the configured base bucket name (without a random suffix).
func EffectiveBucketName(obj *objectstoragev1.Bucket) string {
	if obj.Spec.Name != "" {
		return obj.Spec.Name
	}
	return helpers.EffectiveName(obj.Name, obj.Spec.Metadata)
}

// provisionedBucketName returns the Thalassa bucket name known to belong to this CR.
// When status.bucketName is empty and generateNameSuffix is enabled, the base name must
// not be used: it can identify a different bucket in the organisation.
func provisionedBucketName(obj *objectstoragev1.Bucket) (name string, ok bool) {
	if obj == nil {
		return "", false
	}
	if obj.Status.BucketName != "" {
		return obj.Status.BucketName, true
	}
	if GenerateNameSuffixEnabled(obj) {
		return "", false
	}
	n := EffectiveBucketName(obj)
	return n, n != ""
}

// GenerateNameSuffixEnabled reports whether a random suffix should be appended on create.
// Default is true when the field is unset (matches CRD default).
func GenerateNameSuffixEnabled(obj *objectstoragev1.Bucket) bool {
	if obj == nil || obj.Spec.GenerateNameSuffix == nil {
		return true
	}
	return *obj.Spec.GenerateNameSuffix
}

func (h *Handler) resolveRegion(obj *objectstoragev1.Bucket) (string, error) {
	if obj.Spec.Region != "" {
		return obj.Spec.Region, nil
	}
	if h.DefaultRegion != "" {
		return h.DefaultRegion, nil
	}
	return "", fmt.Errorf("region is required: set spec.region or --default-region")
}

func (h *Handler) createBucket(ctx context.Context, obj *objectstoragev1.Bucket) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	region, err := h.resolveRegion(obj)
	if err != nil {
		return h.setErrorCondition(ctx, obj, "RegionNotResolved", err.Error(), err)
	}

	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating bucket in Thalassa")
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	versioning := objectstorage.ObjectStorageBucketVersioning(obj.Spec.Versioning)
	if versioning == "" {
		versioning = objectstorage.ObjectStorageBucketVersioningDisabled
	}

	baseName := EffectiveBucketName(obj)
	withSuffix := GenerateNameSuffixEnabled(obj)

	var lastErr error
	attempts := 1
	if withSuffix {
		attempts = maxCreateNameAttempts
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		bucketName := baseName
		if withSuffix {
			suffix, genErr := randomNameSuffix()
			if genErr != nil {
				return h.setErrorCondition(ctx, obj, "NameSuffixFailed", genErr.Error(), genErr)
			}
			bucketName = joinBucketName(baseName, suffix)
		}

		createReq := objectstorage.CreateBucketRequest{
			BucketName:        bucketName,
			Public:            obj.Spec.Public,
			Region:            region,
			Labels:            helpers.EffectiveLabels(obj.Spec.Metadata),
			Annotations:       helpers.EffectiveAnnotations(obj.Spec.Metadata),
			Versioning:        versioning,
			ObjectLockEnabled: obj.Spec.ObjectLockEnabled,
		}
		if obj.Spec.Policy != "" {
			var policy objectstorage.PolicyDocument
			if err := json.Unmarshal([]byte(obj.Spec.Policy), &policy); err != nil {
				return h.setErrorCondition(ctx, obj, "InvalidPolicy", fmt.Sprintf("failed to parse spec.policy: %v", err), err)
			}
			createReq.PolicyDocument = &policy
		}

		// Adopt existing bucket by name only when the BucketAdoption feature gate is enabled
		// and the remote bucket carries the required adoption labels.
		existing, getErr := h.ObjectStorage.GetBucket(ctx, bucketName)
		if getErr == nil && existing != nil {
			if withSuffix {
				// Extremely unlikely collision on a fresh random name; try another suffix.
				lastErr = fmt.Errorf("generated bucket name %q already exists", bucketName)
				log.Info("generated bucket name collision, retrying", "name", bucketName, "attempt", attempt)
				continue
			}
			if err := CheckAdoptionEligibility(existing, h.BucketAdoptionEnabled, h.AdoptionRequiredLabels); err != nil {
				return h.setErrorCondition(ctx, obj, "AdoptionDenied", err.Error(), err)
			}
			h.applyObservedStatus(obj, existing)
			obj.Status.LastReconcileError = ""
			h.setConditionFromProviderStatus(obj, existing.Status)
			if err := h.updateStatusWithRetry(ctx, obj); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
			}
			h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Adopted", "Adopted existing Thalassa bucket %s", bucketName)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		if getErr != nil && !thalassaclient.IsNotFound(getErr) {
			return h.setErrorCondition(ctx, obj, "GetFailed", getErr.Error(), getErr)
		}

		created, err := h.ObjectStorage.CreateBucket(ctx, createReq)
		if err != nil {
			lastErr = err
			if withSuffix && attempt < attempts {
				log.Info("create bucket failed, retrying with new suffix", "name", bucketName, "attempt", attempt, "error", err)
				continue
			}
			return h.setErrorCondition(ctx, obj, "FailedCreate", err.Error(), err)
		}

		h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Created", "Created Thalassa bucket %s (%s)", created.Name, created.Identity)
		h.applyObservedStatus(obj, created)
		obj.Status.LastReconcileError = ""
		h.setConditionFromProviderStatus(obj, created.Status)
		if err := h.updateStatusWithRetry(ctx, obj); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
		}
		log.Info("created bucket in Thalassa", "name", created.Name, "identity", created.Identity, "baseName", baseName, "nameSuffix", withSuffix)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("exhausted %d attempts to allocate a unique bucket name for base %q", attempts, baseName)
	}
	return h.setErrorCondition(ctx, obj, "FailedCreate", lastErr.Error(), lastErr)
}

// randomNameSuffix returns a lowercase hex suffix (2 * nameSuffixBytes chars).
func randomNameSuffix() (string, error) {
	b := make([]byte, nameSuffixBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate name suffix: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// joinBucketName returns base-suffix truncated to maxBucketNameLen.
func joinBucketName(base, suffix string) string {
	base = strings.TrimSpace(base)
	suffix = strings.TrimSpace(suffix)
	if base == "" {
		return suffix
	}
	if suffix == "" {
		return truncateBucketName(base)
	}
	joined := base + "-" + suffix
	if len(joined) <= maxBucketNameLen {
		return joined
	}
	// Keep the suffix; trim the base so the result stays unique and valid.
	keep := maxBucketNameLen - len(suffix) - 1
	if keep < 1 {
		return truncateBucketName(suffix)
	}
	return strings.TrimSuffix(base[:keep], "-") + "-" + suffix
}

func truncateBucketName(name string) string {
	if len(name) <= maxBucketNameLen {
		return name
	}
	return name[:maxBucketNameLen]
}

func (h *Handler) reconcileBucket(ctx context.Context, obj *objectstoragev1.Bucket, bucketName string) (ctrl.Result, error) {
	fetched, err := h.ObjectStorage.GetBucket(ctx, bucketName)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			obj.Status.ResourceID = ""
			obj.Status.BucketName = ""
			if err := h.updateStatusWithRetry(ctx, obj); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		return h.setErrorCondition(ctx, obj, "GetFailed", err.Error(), err)
	}

	h.applyObservedStatus(obj, fetched)
	obj.Status.LastReconcileError = ""

	if h.bucketRequiresUpdate(obj, fetched) {
		upd, err := h.buildUpdateRequest(obj, fetched)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "InvalidPolicy", err.Error(), err)
		}
		if _, err := h.ObjectStorage.UpdateBucket(ctx, bucketName, upd); err != nil {
			return h.setErrorCondition(ctx, obj, "FailedUpdate", err.Error(), err)
		}
		h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Updated", "Updated Thalassa bucket %s", bucketName)
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Updating", "Updating bucket in Thalassa")
		if err := h.updateStatusWithRetry(ctx, obj); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	h.setConditionFromProviderStatus(obj, fetched.Status)
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	requeue := 5 * time.Minute
	if !strings.EqualFold(fetched.Status, "ready") && !strings.EqualFold(fetched.Status, "Ready") {
		requeue = 20 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (h *Handler) buildUpdateRequest(obj *objectstoragev1.Bucket, current *objectstorage.ObjectStorageBucket) (objectstorage.UpdateBucketRequest, error) {
	versioning := objectstorage.ObjectStorageBucketVersioning(obj.Spec.Versioning)
	if versioning == "" {
		versioning = objectstorage.ObjectStorageBucketVersioningDisabled
	}
	objectLock := obj.Spec.ObjectLockEnabled
	upd := objectstorage.UpdateBucketRequest{
		Public:            obj.Spec.Public,
		Versioning:        versioning,
		ObjectLockEnabled: &objectLock,
		Labels:            helpers.EffectiveLabels(obj.Spec.Metadata),
		Annotations:       helpers.EffectiveAnnotations(obj.Spec.Metadata),
	}
	if obj.Spec.Policy != "" {
		var policy objectstorage.PolicyDocument
		if err := json.Unmarshal([]byte(obj.Spec.Policy), &policy); err != nil {
			return upd, fmt.Errorf("failed to parse spec.policy: %w", err)
		}
		// Preserve BucketAccess-managed statements so user policy updates do not revoke grants.
		policy = mergePreservingManagedStatements(policy, current.Policy)
		upd.PolicyDocument = &policy
	}
	return upd, nil
}

// mergePreservingManagedStatements copies managed Sid statements from current into desired
// when the desired policy does not already define them.
func mergePreservingManagedStatements(desired, current objectstorage.PolicyDocument) objectstorage.PolicyDocument {
	have := map[string]struct{}{}
	for _, s := range desired.Statement {
		have[s.Sid] = struct{}{}
	}
	for _, s := range current.Statement {
		if !bucketaccess.IsManagedSid(s.Sid) {
			continue
		}
		if _, ok := have[s.Sid]; ok {
			continue
		}
		desired.Statement = append(desired.Statement, s)
	}
	return desired
}

func (h *Handler) bucketRequiresUpdate(obj *objectstoragev1.Bucket, fetched *objectstorage.ObjectStorageBucket) bool {
	if obj.Spec.Public != fetched.Public {
		return true
	}
	wantVersioning := string(obj.Spec.Versioning)
	if wantVersioning == "" {
		wantVersioning = string(objectstoragev1.BucketVersioningDisabled)
	}
	if wantVersioning != string(fetched.Versioning) {
		return true
	}
	if obj.Spec.ObjectLockEnabled != fetched.ObjectLockEnabled {
		return true
	}
	wantLabels := helpers.EffectiveLabels(obj.Spec.Metadata)
	if wantLabels == nil {
		wantLabels = map[string]string{}
	}
	gotLabels := fetched.Labels
	if gotLabels == nil {
		gotLabels = map[string]string{}
	}
	if !equality.Semantic.DeepEqual(wantLabels, gotLabels) {
		return true
	}
	wantAnnotations := helpers.EffectiveAnnotations(obj.Spec.Metadata)
	if wantAnnotations == nil {
		wantAnnotations = map[string]string{}
	}
	gotAnnotations := fetched.Annotations
	if gotAnnotations == nil {
		gotAnnotations = map[string]string{}
	}
	if !equality.Semantic.DeepEqual(wantAnnotations, gotAnnotations) {
		return true
	}
	if obj.Spec.Policy != "" {
		var wantPolicy objectstorage.PolicyDocument
		if err := json.Unmarshal([]byte(obj.Spec.Policy), &wantPolicy); err == nil {
			wantPolicy = mergePreservingManagedStatements(wantPolicy, fetched.Policy)
			if !equality.Semantic.DeepEqual(wantPolicy, fetched.Policy) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) applyObservedStatus(obj *objectstoragev1.Bucket, fetched *objectstorage.ObjectStorageBucket) {
	obj.Status.ResourceID = fetched.Identity
	obj.Status.BucketName = fetched.Name
	obj.Status.Endpoint = fetched.Endpoint
	obj.Status.ResourceStatus = fetched.Status
	obj.Status.Usage = objectstoragev1.BucketUsage{
		TotalSize:    helpers.QuantityFromDecimalGigabytes(fetched.Usage.TotalSizeGB),
		TotalObjects: fetched.Usage.TotalObjects,
	}
	if fetched.Region != nil {
		obj.Status.Region = fetched.Region.Slug
		if obj.Status.Region == "" {
			obj.Status.Region = fetched.Region.Identity
		}
	}
}

func (h *Handler) setConditionFromProviderStatus(obj *objectstoragev1.Bucket, status string) {
	if strings.EqualFold(status, "ready") {
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateAvailable, "Ready", "Bucket is ready")
		return
	}
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Provisioning", fmt.Sprintf("Bucket status: %s", status))
}

func (h *Handler) Terminate(ctx context.Context, obj *objectstoragev1.Bucket) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(obj, Finalizer) {
		return ctrl.Result{}, nil
	}
	name, ok := provisionedBucketName(obj)
	if ok && name != "" {
		if err := h.ObjectStorage.DeleteBucket(ctx, name); err != nil && !thalassaclient.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted bucket in Thalassa", "name", name)
		h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Deleted", "Deleted Thalassa bucket %s", name)
	} else if !ok {
		// Prefer orphaning an unknown remote bucket over deleting by base name.
		log.Info("skipping remote bucket delete: status.bucketName empty with generateNameSuffix enabled")
		h.Recorder.Eventf(obj, corev1.EventTypeWarning, "DeleteSkipped",
			"Skipped Thalassa bucket delete because status.bucketName is empty and generateNameSuffix is enabled (base name may identify a different bucket)")
	}
	if controllerutil.RemoveFinalizer(obj, Finalizer) {
		if err := h.Client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (h *Handler) updateStatusWithRetry(ctx context.Context, obj *objectstoragev1.Bucket) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest objectstoragev1.Bucket
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(obj), &latest); err != nil {
			return err
		}
		latest.Status = obj.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setErrorCondition(ctx context.Context, obj *objectstoragev1.Bucket, reason, message string, err error) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	obj.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, obj); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	// Return the error alone; controller-runtime ignores RequeueAfter when err != nil.
	return ctrl.Result{}, err
}
