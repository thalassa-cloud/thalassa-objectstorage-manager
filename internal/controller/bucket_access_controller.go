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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/thalassa-cloud/client-go/iam"
	"github.com/thalassa-cloud/client-go/objectstorage"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/bucketaccess"
)

// BucketAccessReconciler reconciles a BucketAccess object.
type BucketAccessReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	IAM                         *iam.Client
	ObjectStorage               *objectstorage.Client
	OrganisationID              string
	AllowAllNamespacesSecretRef bool
	Recorder                    record.EventRecorder
	Handler                     *bucketaccess.Handler
}

// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=bucketaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=bucketaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=bucketaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=objectstorage.controllers.thalassa.cloud,resources=buckets,verbs=get;list;watch
// Secrets RBAC is namespaced via the Helm chart (Role/RoleBinding). Do not grant cluster-wide Secrets here.
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *BucketAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var obj objectstoragev1.BucketAccess
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.IAM == nil || r.ObjectStorage == nil {
		log.Info("Thalassa clients not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&obj) {
		return ctrl.Result{}, nil
	}

	obj.Status.ObservedGeneration, obj.Status.LastReconcileTime = ReconcileMeta(obj.Generation)

	if !obj.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &obj)
	}
	if controllerutil.AddFinalizer(&obj, bucketaccess.Finalizer) {
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	return r.Handler.Reconcile(ctx, &obj)
}

func (r *BucketAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("bucketaccess")
	if r.Handler == nil {
		r.Handler = bucketaccess.NewHandler(bucketaccess.Config{
			Client:                      r.Client,
			Scheme:                      r.Scheme,
			IAM:                         r.IAM,
			ObjectStorage:               r.ObjectStorage,
			OrganisationID:              r.OrganisationID,
			AllowAllNamespacesSecretRef: r.AllowAllNamespacesSecretRef,
			Recorder:                    r.Recorder,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&objectstoragev1.BucketAccess{}, builder.WithPredicates(PrimaryResourcePredicate())).
		Owns(&corev1.Secret{}, builder.WithPredicates(OwnedResourcePredicate())).
		Watches(
			&objectstoragev1.Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.mapBucketToAccess),
		).
		Named("bucketaccess").
		Complete(r)
}

func (r *BucketAccessReconciler) mapBucketToAccess(ctx context.Context, obj client.Object) []reconcile.Request {
	bucket, ok := obj.(*objectstoragev1.Bucket)
	if !ok {
		return nil
	}
	var list objectstoragev1.BucketAccessList
	if err := r.List(ctx, &list, client.InNamespace(bucket.Namespace)); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range list.Items {
		access := &list.Items[i]
		for _, ref := range access.Spec.BucketRefs {
			if ref.Name == bucket.Name {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: access.Namespace, Name: access.Name},
				})
				break
			}
		}
	}
	return reqs
}
