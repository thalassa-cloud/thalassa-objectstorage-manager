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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

func TestBucketCustomValidator_ValidateDelete(t *testing.T) {
	t.Parallel()

	validator := &BucketCustomValidator{}

	tests := []struct {
		name    string
		obj     *objectstoragev1.Bucket
		wantErr string
	}{
		{
			name: "nil object allowed",
			obj:  nil,
		},
		{
			name: "deleteProtection false allows delete",
			obj: &objectstoragev1.Bucket{
				Name: "app-data", Namespace: "default",
				Spec: objectstoragev1.BucketSpec{DeleteProtection: false},
			},
		},
		{
			name: "deleteProtection true denies delete",
			obj: &objectstoragev1.Bucket{
				Name: "app-data", Namespace: "default",
				Spec: objectstoragev1.BucketSpec{DeleteProtection: true},
			},
			wantErr: deleteProtectionDeniedMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warnings, err := validator.ValidateDelete(context.Background(), tt.obj)
			assert.Nil(t, warnings)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBucketCustomValidator_ValidateCreateAndUpdate(t *testing.T) {
	t.Parallel()

	validator := &BucketCustomValidator{}
	obj := &objectstoragev1.Bucket{
		Name: "app-data",
		Spec: objectstoragev1.BucketSpec{DeleteProtection: true},
	}

	warnings, err := validator.ValidateCreate(context.Background(), obj)
	assert.Nil(t, warnings)
	require.NoError(t, err)

	warnings, err = validator.ValidateUpdate(context.Background(), obj, obj)
	assert.Nil(t, warnings)
	require.NoError(t, err)
}
