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
	"fmt"
	"strings"

	"github.com/thalassa-cloud/client-go/iam"
	"github.com/thalassa-cloud/client-go/pkg/base"
)

// OrganisationLister lists organisations available to the authenticated principal.
type OrganisationLister interface {
	ListMyOrganisations(ctx context.Context) ([]base.Organisation, error)
}

// ServiceAccountOrganisationReader fetches a service account that may embed its organisation.
type ServiceAccountOrganisationReader interface {
	GetServiceAccount(ctx context.Context, identity string) (*iam.ServiceAccount, error)
}

// ResolveOrganisationIdentity resolves a configured organisation slug or identity to the
// canonical organisation identity. Bucket policy principal ARNs require the identity;
// API headers accept either form.
//
// Resolution order:
//  1. Match identity or slug via ListMyOrganisations.
//  2. If that fails and serviceAccountID is set, derive the organisation from that
//     service account (typical token-exchange bootstrap path).
func ResolveOrganisationIdentity(
	ctx context.Context,
	lister OrganisationLister,
	ref string,
	saReader ServiceAccountOrganisationReader,
	serviceAccountID string,
) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("organisation is required")
	}

	if lister != nil {
		identity, err := resolveFromMemberships(ctx, lister, ref)
		if err == nil {
			return identity, nil
		}
		// Fall through to service-account lookup when memberships are unavailable
		// (common for federated service accounts) or the ref was not found there.
		if saReader == nil || strings.TrimSpace(serviceAccountID) == "" {
			return "", err
		}
		saIdentity, saErr := resolveFromServiceAccount(ctx, saReader, serviceAccountID, ref)
		if saErr != nil {
			return "", fmt.Errorf("%w; service account fallback: %v", err, saErr)
		}
		return saIdentity, nil
	}

	if saReader == nil || strings.TrimSpace(serviceAccountID) == "" {
		return "", fmt.Errorf("organisation %q could not be resolved: no organisation lister configured", ref)
	}
	return resolveFromServiceAccount(ctx, saReader, serviceAccountID, ref)
}

func resolveFromMemberships(ctx context.Context, lister OrganisationLister, ref string) (string, error) {
	orgs, err := lister.ListMyOrganisations(ctx)
	if err != nil {
		return "", fmt.Errorf("list organisations: %w", err)
	}

	var slugMatch string
	for _, org := range orgs {
		identity := strings.TrimSpace(org.Identity)
		if identity == "" {
			continue
		}
		if identity == ref {
			return identity, nil
		}
		if strings.EqualFold(strings.TrimSpace(org.Slug), ref) {
			slugMatch = identity
		}
	}
	if slugMatch != "" {
		return slugMatch, nil
	}
	return "", fmt.Errorf("organisation %q not found among account memberships (use a valid slug or identity)", ref)
}

func resolveFromServiceAccount(
	ctx context.Context,
	saReader ServiceAccountOrganisationReader,
	serviceAccountID, ref string,
) (string, error) {
	saID := strings.TrimSpace(serviceAccountID)
	if saID == "" {
		return "", fmt.Errorf("service account ID is required")
	}
	sa, err := saReader.GetServiceAccount(ctx, saID)
	if err != nil {
		return "", fmt.Errorf("get service account %q: %w", saID, err)
	}
	if sa == nil || sa.Organisation == nil {
		return "", fmt.Errorf("service account %q has no organisation", saID)
	}
	identity := strings.TrimSpace(sa.Organisation.Identity)
	if identity == "" {
		return "", fmt.Errorf("service account %q organisation has empty identity", saID)
	}
	slug := strings.TrimSpace(sa.Organisation.Slug)
	if identity != ref && !strings.EqualFold(slug, ref) {
		return "", fmt.Errorf(
			"service account %q belongs to organisation identity %q (slug %q), which does not match configured organisation %q",
			saID, identity, slug, ref,
		)
	}
	return identity, nil
}
