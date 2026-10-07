/*
Copyright 2026 Thalassa Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    13|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package thalassaclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalassa-cloud/client-go/iam"
	"github.com/thalassa-cloud/client-go/pkg/base"
)

type fakeOrgLister struct {
	orgs []base.Organisation
	err  error
}

func (f *fakeOrgLister) ListMyOrganisations(context.Context) ([]base.Organisation, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.orgs, nil
}

type fakeSAReader struct {
	sa  *iam.ServiceAccount
	err error
}

func (f *fakeSAReader) GetServiceAccount(context.Context, string) (*iam.ServiceAccount, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sa, nil
}

func TestResolveOrganisationIdentity(t *testing.T) {
	t.Parallel()

	orgs := []base.Organisation{
		{Identity: "o-acme", Name: "Acme", Slug: "acme"},
		{Identity: "o-other", Name: "Other", Slug: "other-corp"},
	}

	tests := []struct {
		name             string
		ref              string
		lister           OrganisationLister
		saReader         ServiceAccountOrganisationReader
		serviceAccountID string
		want             string
		wantErrContains  string
	}{
		{
			name:            "empty ref",
			ref:             "  ",
			lister:          &fakeOrgLister{orgs: orgs},
			wantErrContains: "organisation is required",
		},
		{
			name:   "match by identity",
			ref:    "o-acme",
			lister: &fakeOrgLister{orgs: orgs},
			want:   "o-acme",
		},
		{
			name:   "match by slug",
			ref:    "acme",
			lister: &fakeOrgLister{orgs: orgs},
			want:   "o-acme",
		},
		{
			name:   "match by slug case-insensitive",
			ref:    "AcMe",
			lister: &fakeOrgLister{orgs: orgs},
			want:   "o-acme",
		},
		{
			name:            "not found in memberships and no SA fallback",
			ref:             "missing",
			lister:          &fakeOrgLister{orgs: orgs},
			wantErrContains: "not found among account memberships",
		},
		{
			name:   "list error falls back to matching service account org slug",
			ref:    "acme",
			lister: &fakeOrgLister{err: errors.New("forbidden")},
			saReader: &fakeSAReader{
				sa: &iam.ServiceAccount{
					Identity: "sa-1",
					Organisation: &base.Organisation{
						Identity: "o-acme",
						Slug:     "acme",
					},
				},
			},
			serviceAccountID: "sa-1",
			want:             "o-acme",
		},
		{
			name:   "not found in memberships falls back to matching service account identity",
			ref:    "o-acme",
			lister: &fakeOrgLister{orgs: nil},
			saReader: &fakeSAReader{
				sa: &iam.ServiceAccount{
					Identity: "sa-1",
					Organisation: &base.Organisation{
						Identity: "o-acme",
						Slug:     "acme",
					},
				},
			},
			serviceAccountID: "sa-1",
			want:             "o-acme",
		},
		{
			name:   "service account org mismatch",
			ref:    "wrong-slug",
			lister: &fakeOrgLister{err: errors.New("forbidden")},
			saReader: &fakeSAReader{
				sa: &iam.ServiceAccount{
					Identity: "sa-1",
					Organisation: &base.Organisation{
						Identity: "o-acme",
						Slug:     "acme",
					},
				},
			},
			serviceAccountID: "sa-1",
			wantErrContains:  "does not match configured organisation",
		},
		{
			name:             "service account only path",
			ref:              "acme",
			lister:           nil,
			saReader:         &fakeSAReader{sa: &iam.ServiceAccount{Identity: "sa-1", Organisation: &base.Organisation{Identity: "o-acme", Slug: "acme"}}},
			serviceAccountID: "sa-1",
			want:             "o-acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveOrganisationIdentity(context.Background(), tt.lister, tt.ref, tt.saReader, tt.serviceAccountID)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
