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

package helpers

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/conditions"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	RequeueAfterStatusUpdateFailure = 15 * time.Second
	RequeueAfterDependencyNotReady  = 5 * time.Second
)

var ErrDependencyNotReady = errors.New("dependency does not have a resource ID yet")

const ResourceStatusReady = stdconditions.ResourceStatusReady

type ConditionState = stdconditions.ConditionState

const (
	ConditionStateAvailable   = stdconditions.ConditionStateAvailable
	ConditionStateDegraded    = stdconditions.ConditionStateDegraded
	ConditionStateProgressing = stdconditions.ConditionStateProgressing
)

func SetStandardConditions(conds *[]metav1.Condition, state ConditionState, reason, message string) {
	stdconditions.SetStandardConditions(conds, state, reason, message)
}

func NeedStatusUpdate(
	conditions []metav1.Condition,
	newReadyStatus metav1.ConditionStatus,
	newReason, newMessage string,
	newLastErr, newResourceStatus string,
	currentLastErr, currentResourceStatus string,
) bool {
	r := meta.FindStatusCondition(conditions, stdconditions.ConditionTypeReady)
	if r == nil {
		return true
	}
	if r.Status != newReadyStatus || r.Reason != newReason || r.Message != newMessage {
		return true
	}
	if currentLastErr != newLastErr || currentResourceStatus != newResourceStatus {
		return true
	}
	return false
}

func ReconcileMeta(generation int64) (observedGeneration int64, lastReconcileTime *metav1.Time) {
	now := metav1.Now()
	return generation, &now
}

const SuspendAnnotationKey = "objectstorage.controllers.thalassa.cloud/suspend"

func IsSuspended(obj metav1.Object) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	v := obj.GetAnnotations()[SuspendAnnotationKey]
	return strings.EqualFold(v, "true") || v == "1"
}

func EffectiveName(defaultName string, resourceMeta *objectstoragev1.ResourceMetadata) string {
	if resourceMeta != nil && resourceMeta.Name != nil && *resourceMeta.Name != "" {
		return *resourceMeta.Name
	}
	return defaultName
}

func EffectiveLabels(resourceMeta *objectstoragev1.ResourceMetadata) map[string]string {
	if resourceMeta == nil || len(resourceMeta.Labels) == 0 {
		return nil
	}
	return resourceMeta.Labels
}

func EffectiveAnnotations(resourceMeta *objectstoragev1.ResourceMetadata) map[string]string {
	if resourceMeta == nil || len(resourceMeta.Annotations) == 0 {
		return nil
	}
	return resourceMeta.Annotations
}

func QuantityFromDecimalGigabytes(gb float64) resource.Quantity {
	switch {
	case math.IsNaN(gb), math.IsInf(gb, 0), gb <= 0:
		return resource.MustParse("0")
	default:
		s := strconv.FormatFloat(gb, 'f', -1, 64)
		q, err := resource.ParseQuantity(s + "G")
		if err != nil {
			return resource.MustParse("0")
		}
		return q
	}
}

func UpdateStatusWithRetry(doUpdate func() error) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, doUpdate)
}
