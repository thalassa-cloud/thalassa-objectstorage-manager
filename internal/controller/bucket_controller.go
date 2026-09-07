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

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/objectstorage"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/bucket"
)

// BucketReconciler reconciles a Bucket object.
type BucketReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	ObjectStorage          *objectstorage.Client
	DefaultRegion          string
	BucketAdoptionEnabled  bool
	AdoptionRequiredLabels map[string]string
	Recorder               record.EventRecorder
	Handler                *bucket.Handler
}

// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=buckets/finalizers,verbs=update

func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var obj objectstoragev1.Bucket
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.ObjectStorage == nil {
		log.Info("object storage client not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&obj) {
		return ctrl.Result{}, nil
	}

	obj.Status.ObservedGeneration, obj.Status.LastReconcileTime = ReconcileMeta(obj.Generation)

	if !obj.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &obj)
	}
	if controllerutil.AddFinalizer(&obj, bucket.Finalizer) {
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	return r.Handler.Reconcile(ctx, &obj)
}

func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("bucket") //nolint:staticcheck // SA1019: handlers use record.EventRecorder; events API uses a different Eventf signature
	if r.Handler == nil {
		r.Handler = bucket.NewHandler(bucket.Config{
			Client:                 r.Client,
			Scheme:                 r.Scheme,
			ObjectStorage:          r.ObjectStorage,
			DefaultRegion:          r.DefaultRegion,
			BucketAdoptionEnabled:  r.BucketAdoptionEnabled,
			AdoptionRequiredLabels: r.AdoptionRequiredLabels,
			Recorder:               r.Recorder,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&objectstoragev1.Bucket{}, builder.WithPredicates(PrimaryResourcePredicate())).
		Named("bucket").
		Complete(r)
}
