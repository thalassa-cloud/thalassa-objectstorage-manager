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
	"github.com/stretchr/testify/require"
)

func TestResolvePermissions(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		custom  []string
		want    []string
		wantErr string
	}{
		{
			name: "default readwrite",
			want: ReadWritePermissions,
		},
		{
			name:   "explicit readwrite preset",
			preset: PresetReadWrite,
			want:   ReadWritePermissions,
		},
		{
			name:   "readonly preset",
			preset: PresetReadOnly,
			want:   ReadOnlyPermissions,
		},
		{
			name:   "custom allowlisted subset",
			custom: []string{"s3:GetObject", "s3:ListBucket"},
			want:   []string{"s3:GetObject", "s3:ListBucket"},
		},
		{
			name:    "rejects wildcard",
			custom:  []string{"s3:*"},
			wantErr: "disallowed",
		},
		{
			name:    "rejects PutBucketPolicy",
			custom:  []string{"s3:PutBucketPolicy"},
			wantErr: "disallowed",
		},
		{
			name:    "rejects DeleteBucket",
			custom:  []string{"s3:DeleteBucket"},
			wantErr: "disallowed",
		},
		{
			name:    "unknown preset",
			preset:  "Admin",
			wantErr: "unknown permissionPreset",
		},
		{
			name:   "custom wins over preset",
			preset: PresetReadOnly,
			custom: []string{"s3:PutObject"},
			want:   []string{"s3:PutObject"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePermissions(tt.preset, tt.custom)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWantsCredentialRotation(t *testing.T) {
	assert.False(t, WantsCredentialRotation(nil))
	assert.False(t, WantsCredentialRotation(map[string]string{}))
	assert.True(t, WantsCredentialRotation(map[string]string{RotateCredentialsAnnotation: "true"}))
	assert.True(t, WantsCredentialRotation(map[string]string{RotateCredentialsAnnotation: "1"}))
	assert.False(t, WantsCredentialRotation(map[string]string{RotateCredentialsAnnotation: "false"}))
}
