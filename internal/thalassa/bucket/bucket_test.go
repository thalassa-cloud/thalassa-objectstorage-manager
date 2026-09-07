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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/thalassa-cloud/client-go/objectstorage"
	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

func TestEffectiveBucketName(t *testing.T) {
	nameOverride := "from-metadata"
	tests := []struct {
		name string
		obj  *objectstoragev1.Bucket
		want string
	}{
		{
			name: "spec name wins",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "cr-name"},
				Spec:       objectstoragev1.BucketSpec{Name: "explicit"},
			},
			want: "explicit",
		},
		{
			name: "metadata name when spec empty",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "cr-name"},
				Spec: objectstoragev1.BucketSpec{
					Metadata: &objectstoragev1.ResourceMetadata{Name: &nameOverride},
				},
			},
			want: "from-metadata",
		},
		{
			name: "falls back to object name",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "cr-name"},
			},
			want: "cr-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EffectiveBucketName(tt.obj))
		})
	}
}

func TestProvisionedBucketName(t *testing.T) {
	falseVal := false
	trueVal := true
	tests := []struct {
		name   string
		obj    *objectstoragev1.Bucket
		want   string
		wantOK bool
	}{
		{
			name:   "status bucket name wins",
			obj:    &objectstoragev1.Bucket{Status: objectstoragev1.BucketStatus{BucketName: "logs-a1b2c3"}},
			want:   "logs-a1b2c3",
			wantOK: true,
		},
		{
			name: "empty status with suffix enabled refuses base name",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "logs"},
				Spec:       objectstoragev1.BucketSpec{Name: "logs", GenerateNameSuffix: &trueVal},
			},
			want:   "",
			wantOK: false,
		},
		{
			name: "empty status with suffix default (true) refuses base name",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "logs"},
				Spec:       objectstoragev1.BucketSpec{Name: "logs"},
			},
			want:   "",
			wantOK: false,
		},
		{
			name: "empty status with suffix disabled uses base name",
			obj: &objectstoragev1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "logs"},
				Spec:       objectstoragev1.BucketSpec{Name: "logs", GenerateNameSuffix: &falseVal},
			},
			want:   "logs",
			wantOK: true,
		},
		{name: "nil object", obj: nil, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := provisionedBucketName(tt.obj)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateNameSuffixEnabled(t *testing.T) {
	falseVal := false
	trueVal := true
	tests := []struct {
		name string
		obj  *objectstoragev1.Bucket
		want bool
	}{
		{name: "nil defaults to true", obj: &objectstoragev1.Bucket{}, want: true},
		{name: "explicit true", obj: &objectstoragev1.Bucket{Spec: objectstoragev1.BucketSpec{GenerateNameSuffix: &trueVal}}, want: true},
		{name: "explicit false", obj: &objectstoragev1.Bucket{Spec: objectstoragev1.BucketSpec{GenerateNameSuffix: &falseVal}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GenerateNameSuffixEnabled(tt.obj))
		})
	}
}

func TestJoinBucketName(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		suffix string
		want   string
	}{
		{name: "simple", base: "app-data", suffix: "a1b2c3", want: "app-data-a1b2c3"},
		{name: "empty suffix", base: "app-data", suffix: "", want: "app-data"},
		{name: "empty base", base: "", suffix: "a1b2c3", want: "a1b2c3"},
		{
			name:   "truncates long base",
			base:   strings.Repeat("a", 70),
			suffix: "aabbcc",
			want:   strings.Repeat("a", 56) + "-aabbcc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinBucketName(tt.base, tt.suffix)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), maxBucketNameLen)
		})
	}
}

func TestRandomNameSuffix(t *testing.T) {
	a, err := randomNameSuffix()
	require.NoError(t, err)
	b, err := randomNameSuffix()
	require.NoError(t, err)
	assert.Len(t, a, nameSuffixBytes*2)
	assert.Len(t, b, nameSuffixBytes*2)
	assert.NotEqual(t, a, b)
	assert.Regexp(t, `^[0-9a-f]+$`, a)
}

func TestMergePreservingManagedStatements(t *testing.T) {
	managedSid := "tosm-default-access"
	desired := objectstorage.PolicyDocument{
		Statement: []objectstorage.Statement{{Sid: "user", Effect: "Allow"}},
	}
	current := objectstorage.PolicyDocument{
		Statement: []objectstorage.Statement{
			{Sid: managedSid, Effect: "Allow", Action: "s3:GetObject"},
			{Sid: "other", Effect: "Deny"},
		},
	}
	got := mergePreservingManagedStatements(desired, current)
	require.Len(t, got.Statement, 2)
	assert.Equal(t, "user", got.Statement[0].Sid)
	assert.Equal(t, managedSid, got.Statement[1].Sid)
}
