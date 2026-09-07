/*
Copyright 2026 Thalassa Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    15|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bucketaccess

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/thalassa-cloud/client-go/objectstorage"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
)

const (
	// ManagedSidPrefix identifies policy statements owned by this controller.
	// Sid values must be alphanumeric (plus hyphen); '/' is rejected by the Thalassa API.
	ManagedSidPrefix = "tosm-"

	// legacyManagedSidPrefix was used before Sid validation rejected '/'.
	legacyManagedSidPrefix = "thalassa-objectstorage-manager/"

	policyVersion = "2012-10-17"
)

// ManagedSid builds the Sid for a BucketAccess resource.
// Format: tosm-<namespace>-<name> with non [A-Za-z0-9-] characters replaced by '-'.
func ManagedSid(namespace, name string) string {
	return ManagedSidPrefix + sanitizeSidPart(namespace) + "-" + sanitizeSidPart(name)
}

func sanitizeSidPart(s string) string {
	if s == "" {
		return "x"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ServiceAccountPrincipalARN builds the Thalassa IAM principal ARN for a service account.
func ServiceAccountPrincipalARN(organisationID, serviceAccountID string) string {
	return fmt.Sprintf("arn:thalassa:iam:::serviceaccount/%s:%s", organisationID, serviceAccountID)
}

// UserPrincipalARN builds the Thalassa IAM principal ARN for a user.
func UserPrincipalARN(organisationID, userID string) string {
	return fmt.Sprintf("arn:thalassa:iam:::user/%s:%s", organisationID, userID)
}

// PrincipalARN builds the ARN for a PrincipalReference.
func PrincipalARN(organisationID string, ref objectstoragev1.PrincipalReference) (string, error) {
	id := strings.TrimSpace(ref.Identity)
	if id == "" {
		return "", fmt.Errorf("principalRef.identity is required")
	}
	org := strings.TrimSpace(organisationID)
	if org == "" {
		return "", fmt.Errorf("organisation ID is required for principal ARNs")
	}
	switch ref.Kind {
	case objectstoragev1.PrincipalKindServiceAccount:
		return ServiceAccountPrincipalARN(org, id), nil
	case objectstoragev1.PrincipalKindUser:
		return UserPrincipalARN(org, id), nil
	default:
		return "", fmt.Errorf("unsupported principalRef.kind %q (want User or ServiceAccount)", ref.Kind)
	}
}

// IsManagedMode reports whether BucketAccess creates/manages IAM credentials (no principalRef).
func IsManagedMode(obj *objectstoragev1.BucketAccess) bool {
	return obj == nil || obj.Spec.PrincipalRef == nil
}

// BucketARN builds the Thalassa S3 bucket ARN.
func BucketARN(bucketName string) string {
	return fmt.Sprintf("arn:thalassa:s3:::%s", bucketName)
}

// UpsertManagedStatement merges (or inserts) the managed Allow statement into the policy.
// Other statements are preserved.
func UpsertManagedStatement(policy objectstorage.PolicyDocument, sid, principalARN, bucketName string, actions []string) objectstorage.PolicyDocument {
	if policy.Version == "" {
		policy.Version = policyVersion
	}
	if actions == nil {
		actions = DefaultPermissions
	}
	stmt := objectstorage.Statement{
		Sid:    sid,
		Effect: "Allow",
		Principal: objectstorage.Principal{
			Thalassa: []string{principalARN},
		},
		Action: actions,
		Resource: []string{
			BucketARN(bucketName),
			BucketARN(bucketName) + "/*",
		},
	}

	replaced := false
	out := make([]objectstorage.Statement, 0, len(policy.Statement)+1)
	for _, s := range policy.Statement {
		if s.Sid == sid {
			out = append(out, stmt)
			replaced = true
			continue
		}
		out = append(out, s)
	}
	if !replaced {
		out = append(out, stmt)
	}
	policy.Statement = out
	return policy
}

// RemoveManagedStatement removes the statement with the given Sid.
func RemoveManagedStatement(policy objectstorage.PolicyDocument, sid string) objectstorage.PolicyDocument {
	out := make([]objectstorage.Statement, 0, len(policy.Statement))
	for _, s := range policy.Statement {
		if s.Sid == sid {
			continue
		}
		out = append(out, s)
	}
	policy.Statement = out
	return policy
}

// IsManagedSid reports whether sid was created by this controller (current or legacy format).
func IsManagedSid(sid string) bool {
	return strings.HasPrefix(sid, ManagedSidPrefix) || strings.HasPrefix(sid, legacyManagedSidPrefix)
}
