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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/thalassa-cloud/client-go/iam"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

func TestSecretOwnedBy(t *testing.T) {
	uid := types.UID("uid-1")
	ctrl := true
	obj := &objectstoragev1.BucketAccess{
		ObjectMeta: metav1.ObjectMeta{Name: "access", Namespace: "app", UID: uid},
	}

	tests := []struct {
		name   string
		secret *corev1.Secret
		want   bool
	}{
		{
			name:   "unowned secret",
			secret: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "app"}},
			want:   false,
		},
		{
			name: "controller ownerRef",
			secret: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "s", Namespace: "app",
				OwnerReferences: []metav1.OwnerReference{{
					UID: uid, Kind: "BucketAccess", Controller: &ctrl,
				}},
			}},
			want: true,
		},
		{
			name: "cross-namespace ownership label",
			secret: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "s", Namespace: "secrets",
				Labels: map[string]string{ownershipLabelKey: "app.access"},
			}},
			want: true,
		},
		{
			name: "wrong ownership label",
			secret: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "s", Namespace: "secrets",
				Labels: map[string]string{ownershipLabelKey: "other.access"},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, secretOwnedBy(obj, tt.secret))
		})
	}
}

func TestServiceAccountOwnedBy(t *testing.T) {
	obj := &objectstoragev1.BucketAccess{
		ObjectMeta: metav1.ObjectMeta{Name: "access", Namespace: "app"},
	}
	tests := []struct {
		name string
		sa   *iam.ServiceAccount
		want bool
	}{
		{name: "nil sa", sa: nil, want: false},
		{name: "no labels", sa: &iam.ServiceAccount{Identity: "sa-1"}, want: false},
		{
			name: "owned",
			sa: &iam.ServiceAccount{
				Identity: "sa-1",
				Labels:   map[string]string{ownershipLabelKey: "app.access"},
			},
			want: true,
		},
		{
			name: "external principal leftover in status",
			sa: &iam.ServiceAccount{
				Identity: "external-sa",
				Labels:   map[string]string{"team": "platform"},
			},
			want: false,
		},
		{
			name: "wrong owner",
			sa: &iam.ServiceAccount{
				Identity: "sa-2",
				Labels:   map[string]string{ownershipLabelKey: "other.access"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, serviceAccountOwnedBy(obj, tt.sa))
		})
	}
}
