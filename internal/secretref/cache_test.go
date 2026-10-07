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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestParseNamespaces(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{
			name: "empty",
			raw:  "",
			want: nil,
		},
		{
			name: "whitespace only",
			raw:  "  , , ",
			want: nil,
		},
		{
			name: "single",
			raw:  "apps",
			want: []string{"apps"},
		},
		{
			name: "trims and drops empties",
			raw:  " apps, ,default, ",
			want: []string{"apps", "default"},
		},
		{
			name: "deduplicates and sorts",
			raw:  "zeta,apps,apps,default",
			want: []string{"apps", "default", "zeta"},
		},
		{
			name:    "rejects invalid namespace",
			raw:     "apps,Not_Valid",
			wantErr: true,
		},
		{
			name:    "rejects path-like input",
			raw:     "apps/../kube-system",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNamespaces(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSecretCacheOptions(t *testing.T) {
	tests := []struct {
		name               string
		allowAllNamespaces bool
		namespaces         []string
		wantClusterScoped  bool
		wantNamespaces     []string
	}{
		{
			name:              "empty namespaces stays cluster-scoped",
			wantClusterScoped: true,
		},
		{
			name:               "allow-all ignores listed namespaces",
			allowAllNamespaces: true,
			namespaces:         []string{"apps", "default"},
			wantClusterScoped:  true,
		},
		{
			name:           "restricts secret informer to listed namespaces",
			namespaces:     []string{"apps", "default"},
			wantNamespaces: []string{"apps", "default"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SecretCacheOptions(tt.allowAllNamespaces, tt.namespaces)
			if tt.wantClusterScoped {
				assert.Empty(t, opts.ByObject)
				return
			}
			require.NotEmpty(t, opts.ByObject)
			var got []string
			for obj, by := range opts.ByObject {
				_, isSecret := obj.(*corev1.Secret)
				require.True(t, isSecret, "expected Secret ByObject, got %T", obj)
				for ns := range by.Namespaces {
					got = append(got, ns)
				}
			}
			assert.ElementsMatch(t, tt.wantNamespaces, got)
			assert.Len(t, opts.ByObject, 1)
		})
	}
}
