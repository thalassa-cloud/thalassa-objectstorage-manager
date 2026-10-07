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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/thalassa-cloud/client-go/iam"
	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

func TestManagedServiceAccountName(t *testing.T) {
	tests := []struct {
		name string
		obj  *objectstoragev1.BucketAccess
		want string
	}{
		{
			name: "explicit service account name",
			obj: &objectstoragev1.BucketAccess{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "access"},
				Spec:       objectstoragev1.BucketAccessSpec{ServiceAccountName: "custom-sa"},
			},
			want: "custom-sa",
		},
		{
			name: "defaults to namespace-name",
			obj: &objectstoragev1.BucketAccess{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "access"},
			},
			want: "app-access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, managedServiceAccountName(tt.obj))
		})
	}
}

func TestPickOwnedServiceAccountID(t *testing.T) {
	obj := &objectstoragev1.BucketAccess{
		ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "access"},
	}
	owned := ownershipLabelValue(obj)

	tests := []struct {
		name     string
		accounts []iam.ServiceAccount
		wantName string
		want     string
	}{
		{
			name:     "empty list",
			accounts: nil,
			wantName: "app-access",
			want:     "",
		},
		{
			name: "ignores unowned accounts",
			accounts: []iam.ServiceAccount{
				{Identity: "sa-other", Name: "app-access", Labels: map[string]string{ownershipLabelKey: "other.access"}},
			},
			wantName: "app-access",
			want:     "",
		},
		{
			name: "prefers exact name match",
			accounts: []iam.ServiceAccount{
				{Identity: "sa-fallback", Name: "older-name", Labels: map[string]string{ownershipLabelKey: owned}},
				{Identity: "sa-exact", Name: "app-access", Labels: map[string]string{ownershipLabelKey: owned}},
			},
			wantName: "app-access",
			want:     "sa-exact",
		},
		{
			name: "falls back to first owned when name differs",
			accounts: []iam.ServiceAccount{
				{Identity: "sa-first", Name: "legacy", Labels: map[string]string{ownershipLabelKey: owned}},
				{Identity: "sa-second", Name: "also-legacy", Labels: map[string]string{ownershipLabelKey: owned}},
			},
			wantName: "app-access",
			want:     "sa-first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pickOwnedServiceAccountID(tt.accounts, obj, tt.wantName))
		})
	}
}
