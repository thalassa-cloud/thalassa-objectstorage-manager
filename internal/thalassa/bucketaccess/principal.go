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
	"context"
	"fmt"
	"strings"

	"github.com/thalassa-cloud/client-go/iam"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

// resolvedPrincipal is the outcome of resolving an external or managed principal.
type resolvedPrincipal struct {
	ARN              string
	ServiceAccountID string // set for ServiceAccount principals (managed or external)
}

// resolveExternalPrincipal validates that the referenced principal exists and builds its ARN.
func (h *Handler) resolveExternalPrincipal(ctx context.Context, ref *objectstoragev1.PrincipalReference) (resolvedPrincipal, error) {
	if ref == nil {
		return resolvedPrincipal{}, fmt.Errorf("principalRef is required")
	}
	arn, err := PrincipalARN(h.OrganisationID, *ref)
	if err != nil {
		return resolvedPrincipal{}, err
	}
	id := strings.TrimSpace(ref.Identity)

	switch ref.Kind {
	case objectstoragev1.PrincipalKindServiceAccount:
		sa, err := h.IAM.GetServiceAccount(ctx, id)
		if err != nil {
			if thalassaclient.IsNotFound(err) {
				return resolvedPrincipal{}, fmt.Errorf("service account %q not found", id)
			}
			return resolvedPrincipal{}, fmt.Errorf("get service account %q: %w", id, err)
		}
		if sa == nil || sa.Identity == "" {
			return resolvedPrincipal{}, fmt.Errorf("service account %q not found", id)
		}
		return resolvedPrincipal{ARN: arn, ServiceAccountID: sa.Identity}, nil

	case objectstoragev1.PrincipalKindUser:
		if err := h.ensureUserExists(ctx, id); err != nil {
			return resolvedPrincipal{}, err
		}
		return resolvedPrincipal{ARN: arn}, nil

	default:
		return resolvedPrincipal{}, fmt.Errorf("unsupported principalRef.kind %q", ref.Kind)
	}
}

func (h *Handler) ensureUserExists(ctx context.Context, userSubject string) error {
	members, err := h.IAM.ListOrganisationMembers(ctx, &iam.ListMembersRequest{})
	if err != nil {
		return fmt.Errorf("list organisation members: %w", err)
	}
	for _, m := range members {
		if m.User == nil {
			continue
		}
		if strings.TrimSpace(m.User.Subject) == userSubject {
			return nil
		}
	}
	return fmt.Errorf("user %q not found among organisation members", userSubject)
}
