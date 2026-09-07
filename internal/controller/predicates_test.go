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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	thalassahelpers "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/helpers"
)

func TestPrimaryResourcePredicate_Update(t *testing.T) {
	t.Parallel()

	pred := PrimaryResourcePredicate()
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name string
		old  *objectstoragev1.Bucket
		new  *objectstoragev1.Bucket
		want bool
	}{
		{
			name: "status-only update ignored",
			old: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     objectstoragev1.BucketStatus{Endpoint: ""},
			},
			new: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     objectstoragev1.BucketStatus{Endpoint: "https://s3.example"},
			},
			want: false,
		},
		{
			name: "spec generation change triggers",
			old:  &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new:  &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{Generation: 2}},
			want: true,
		},
		{
			name: "deletion timestamp triggers",
			old:  &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new: &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{
				Generation:        1,
				DeletionTimestamp: &now,
			}},
			want: true,
		},
		{
			name: "suspend annotation added triggers",
			old:  &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new: &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
				Annotations: map[string]string{
					thalassahelpers.SuspendAnnotationKey: "true",
				},
			}},
			want: true,
		},
		{
			name: "resume suspend removed triggers",
			old: &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
				Annotations: map[string]string{
					thalassahelpers.SuspendAnnotationKey: "true",
				},
			}},
			new:  &objectstoragev1.Bucket{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pred.Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new})
			assert.Equal(t, tt.want, got)
		})
	}
}
