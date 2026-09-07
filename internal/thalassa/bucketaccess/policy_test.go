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
	"github.com/thalassa-cloud/client-go/objectstorage"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

func TestManagedSid(t *testing.T) {
	assert.Equal(t, "tosm-default-my-access", ManagedSid("default", "my-access"))
	assert.Equal(t, "tosm-objectstorage-hack-hack-demo-ro", ManagedSid("objectstorage-hack", "hack-demo-ro"))
	assert.Equal(t, "tosm-ns-with-dots-name", ManagedSid("ns.with.dots", "name"))
	assert.True(t, IsManagedSid(ManagedSid("ns", "name")))
	assert.True(t, IsManagedSid("thalassa-objectstorage-manager/default/access"))
	assert.False(t, IsManagedSid("user-owned-sid"))
}

func TestServiceAccountPrincipalARN(t *testing.T) {
	assert.Equal(t,
		"arn:thalassa:iam:::serviceaccount/org-1:sa-2",
		ServiceAccountPrincipalARN("org-1", "sa-2"),
	)
}

func TestUserPrincipalARN(t *testing.T) {
	assert.Equal(t,
		"arn:thalassa:iam:::user/org-1:u-2",
		UserPrincipalARN("org-1", "u-2"),
	)
}

func TestPrincipalARN(t *testing.T) {
	tests := []struct {
		name    string
		org     string
		ref     objectstoragev1.PrincipalReference
		want    string
		wantErr bool
	}{
		{
			name: "service account",
			org:  "org-1",
			ref:  objectstoragev1.PrincipalReference{Kind: objectstoragev1.PrincipalKindServiceAccount, Identity: "sa-abc"},
			want: "arn:thalassa:iam:::serviceaccount/org-1:sa-abc",
		},
		{
			name: "user",
			org:  "org-1",
			ref:  objectstoragev1.PrincipalReference{Kind: objectstoragev1.PrincipalKindUser, Identity: "u-xyz"},
			want: "arn:thalassa:iam:::user/org-1:u-xyz",
		},
		{
			name:    "empty identity",
			org:     "org-1",
			ref:     objectstoragev1.PrincipalReference{Kind: objectstoragev1.PrincipalKindUser, Identity: "  "},
			wantErr: true,
		},
		{
			name:    "bad kind",
			org:     "org-1",
			ref:     objectstoragev1.PrincipalReference{Kind: "Team", Identity: "t-1"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrincipalARN(tt.org, tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsManagedMode(t *testing.T) {
	assert.True(t, IsManagedMode(&objectstoragev1.BucketAccess{}))
	assert.True(t, IsManagedMode(&objectstoragev1.BucketAccess{
		Spec: objectstoragev1.BucketAccessSpec{
			WriteSecretToRef: &objectstoragev1.SecretReference{Name: "s"},
		},
	}))
	assert.False(t, IsManagedMode(&objectstoragev1.BucketAccess{
		Spec: objectstoragev1.BucketAccessSpec{
			PrincipalRef: &objectstoragev1.PrincipalReference{
				Kind:     objectstoragev1.PrincipalKindUser,
				Identity: "u-1",
			},
		},
	}))
}

func TestBucketARN(t *testing.T) {
	assert.Equal(t, "arn:thalassa:s3:::my-bucket", BucketARN("my-bucket"))
}

func TestUpsertManagedStatement(t *testing.T) {
	tests := []struct {
		name       string
		policy     objectstorage.PolicyDocument
		sid        string
		wantLen    int
		wantAction []string
	}{
		{
			name:    "empty policy inserts statement",
			policy:  objectstorage.PolicyDocument{},
			sid:     ManagedSid("default", "access"),
			wantLen: 1,
		},
		{
			name: "preserves unrelated statements",
			policy: objectstorage.PolicyDocument{
				Version: "2012-10-17",
				Statement: []objectstorage.Statement{
					{Sid: "keep-me", Effect: "Allow", Action: "s3:GetObject"},
				},
			},
			sid:     ManagedSid("default", "access"),
			wantLen: 2,
		},
		{
			name: "replaces existing managed sid",
			policy: objectstorage.PolicyDocument{
				Version: "2012-10-17",
				Statement: []objectstorage.Statement{
					{
						Sid:    ManagedSid("default", "access"),
						Effect: "Allow",
						Action: []string{"s3:GetObject"},
					},
				},
			},
			sid:        ManagedSid("default", "access"),
			wantLen:    1,
			wantAction: []string{"s3:PutObject"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := DefaultPermissions
			if tt.wantAction != nil {
				actions = tt.wantAction
			}
			got := UpsertManagedStatement(tt.policy, tt.sid, ServiceAccountPrincipalARN("org", "sa"), "bucket-a", actions)
			require.Len(t, got.Statement, tt.wantLen)
			assert.Equal(t, "2012-10-17", got.Version)

			var managed *objectstorage.Statement
			for i := range got.Statement {
				if got.Statement[i].Sid == tt.sid {
					managed = &got.Statement[i]
					break
				}
			}
			require.NotNil(t, managed)
			assert.Equal(t, "Allow", managed.Effect)
			assert.Equal(t, []string{
				"arn:thalassa:s3:::bucket-a",
				"arn:thalassa:s3:::bucket-a/*",
			}, managed.Resource)
			if tt.wantAction != nil {
				assert.Equal(t, tt.wantAction, managed.Action)
			}
		})
	}
}

func TestRemoveManagedStatement(t *testing.T) {
	sid := ManagedSid("default", "access")
	policy := objectstorage.PolicyDocument{
		Statement: []objectstorage.Statement{
			{Sid: "keep", Effect: "Allow"},
			{Sid: sid, Effect: "Allow"},
		},
	}
	got := RemoveManagedStatement(policy, sid)
	require.Len(t, got.Statement, 1)
	assert.Equal(t, "keep", got.Statement[0].Sid)
}
