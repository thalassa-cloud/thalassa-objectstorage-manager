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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalassa-cloud/client-go/objectstorage"
)

func TestCheckAdoptionEligibility(t *testing.T) {
	labeled := &objectstorage.ObjectStorageBucket{
		Name: "existing",
		Labels: map[string]string{
			AdoptableLabelKey: AdoptableLabelValue,
		},
	}
	unlabeled := &objectstorage.ObjectStorageBucket{Name: "existing"}

	tests := []struct {
		name    string
		bucket  *objectstorage.ObjectStorageBucket
		enabled bool
		wantErr string
	}{
		{
			name:    "disabled rejects existing bucket",
			bucket:  labeled,
			enabled: false,
			wantErr: "adoption is disabled",
		},
		{
			name:    "enabled without labels rejected",
			bucket:  unlabeled,
			enabled: true,
			wantErr: "missing required adoption label",
		},
		{
			name:    "enabled with labels allowed",
			bucket:  labeled,
			enabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckAdoptionEligibility(tt.bucket, tt.enabled, DefaultAdoptionRequiredLabels())
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseLabelPairs(t *testing.T) {
	got, err := ParseLabelPairs("")
	require.NoError(t, err)
	assert.Equal(t, DefaultAdoptionRequiredLabels(), got)

	got, err = ParseLabelPairs("a=1,b=2")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, got)

	_, err = ParseLabelPairs("novalue")
	require.Error(t, err)
}
