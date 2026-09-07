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

package secretref

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name               string
		resourceNamespace  string
		refNamespace       string
		allowAllNamespaces bool
		want               string
		wantErr            error
	}{
		{
			name:              "empty ref defaults to resource namespace",
			resourceNamespace: "app",
			refNamespace:      "",
			want:              "app",
		},
		{
			name:              "same namespace allowed",
			resourceNamespace: "app",
			refNamespace:      "app",
			want:              "app",
		},
		{
			name:               "cross namespace forbidden by default",
			resourceNamespace:  "app",
			refNamespace:       "other",
			allowAllNamespaces: false,
			wantErr:            ErrCrossNamespaceForbidden,
		},
		{
			name:               "cross namespace allowed when enabled",
			resourceNamespace:  "app",
			refNamespace:       "other",
			allowAllNamespaces: true,
			want:               "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.resourceNamespace, tt.refNamespace, tt.allowAllNamespaces)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
