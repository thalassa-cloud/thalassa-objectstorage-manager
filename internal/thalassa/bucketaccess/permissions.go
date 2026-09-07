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
	"fmt"
	"sort"
	"strings"
)

// Permission presets expand to fixed S3 action sets. Custom permissions must be a subset of AllowedActions.
const (
	PresetReadOnly  = "ReadOnly"
	PresetReadWrite = "ReadWrite"

	actionGetObject                  = "s3:GetObject"
	actionGetObjectVersion           = "s3:GetObjectVersion"
	actionPutObject                  = "s3:PutObject"
	actionDeleteObject               = "s3:DeleteObject"
	actionDeleteObjectVersion        = "s3:DeleteObjectVersion"
	actionListMultipartUploadParts   = "s3:ListMultipartUploadParts"
	actionAbortMultipartUpload       = "s3:AbortMultipartUpload"
	actionListBucket                 = "s3:ListBucket"
	actionListBucketVersions         = "s3:ListBucketVersions"
	actionListBucketMultipartUploads = "s3:ListBucketMultipartUploads"
	actionGetBucketAcl               = "s3:GetBucketAcl"
	actionGetBucketVersioning        = "s3:GetBucketVersioning"
	actionGetLifecycleConfiguration  = "s3:GetLifecycleConfiguration"
	actionGetObjectTagging           = "s3:GetObjectTagging"
	actionPutObjectTagging           = "s3:PutObjectTagging"
	actionDeleteObjectTagging        = "s3:DeleteObjectTagging"
	actionGetObjectVersionTagging    = "s3:GetObjectVersionTagging"
	actionPutObjectVersionTagging    = "s3:PutObjectVersionTagging"
	actionDeleteObjectVersionTagging = "s3:DeleteObjectVersionTagging"
	actionGetBucketLocation          = "s3:GetBucketLocation"
	actionGetBucketPolicyStatus      = "s3:GetBucketPolicyStatus"
)

// ReadOnlyPermissions is the least-privilege object read + list set.
var ReadOnlyPermissions = []string{
	actionGetObject,
	actionGetObjectVersion,
	actionListBucket,
	actionListBucketVersions,
	actionGetObjectTagging,
	actionGetObjectVersionTagging,
	actionGetBucketVersioning,
}

// ReadWritePermissions is the default grant set (objects + list + tagging + multipart).
var ReadWritePermissions = []string{
	actionGetObject,
	actionGetObjectVersion,
	actionPutObject,
	actionDeleteObject,
	actionDeleteObjectVersion,
	actionListMultipartUploadParts,
	actionAbortMultipartUpload,
	actionListBucket,
	actionListBucketVersions,
	actionListBucketMultipartUploads,
	actionGetBucketAcl,
	actionGetBucketVersioning,
	actionGetLifecycleConfiguration,
	actionGetObjectTagging,
	actionPutObjectTagging,
	actionDeleteObjectTagging,
	actionGetObjectVersionTagging,
	actionPutObjectVersionTagging,
	actionDeleteObjectVersionTagging,
}

// DefaultPermissions is an alias for ReadWritePermissions (used when preset and permissions are empty).
var DefaultPermissions = ReadWritePermissions

// AllowedActions is the union of all actions that may appear in spec.permissions.
// Dangerous admin actions (DeleteBucket, PutBucketPolicy, *, …) are intentionally excluded.
var AllowedActions = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ReadWritePermissions)+8)
	for _, a := range ReadWritePermissions {
		m[a] = struct{}{}
	}
	// Extra allowlisted read helpers not in the default RW preset.
	for _, a := range []string{
		actionGetObjectVersionTagging,
		actionGetBucketLocation,
		actionGetBucketPolicyStatus,
	} {
		m[a] = struct{}{}
	}
	return m
}()

// ResolvePermissions returns the S3 actions to grant.
// If custom is non-empty, each action must be in AllowedActions (preset is ignored).
// If custom is empty, preset is used (default ReadWrite).
func ResolvePermissions(preset string, custom []string) ([]string, error) {
	if len(custom) > 0 {
		seen := make(map[string]struct{}, len(custom))
		out := make([]string, 0, len(custom))
		var invalid []string
		for _, a := range custom {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if _, ok := AllowedActions[a]; !ok {
				invalid = append(invalid, a)
				continue
			}
			if _, dup := seen[a]; dup {
				continue
			}
			seen[a] = struct{}{}
			out = append(out, a)
		}
		if len(invalid) > 0 {
			sort.Strings(invalid)
			return nil, fmt.Errorf("permissions contain disallowed S3 action(s): %s (use a PermissionPreset or an allowlisted action)", strings.Join(invalid, ", "))
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("permissions is empty after validation")
		}
		return out, nil
	}

	switch strings.TrimSpace(preset) {
	case "", PresetReadWrite:
		return append([]string(nil), ReadWritePermissions...), nil
	case PresetReadOnly:
		return append([]string(nil), ReadOnlyPermissions...), nil
	default:
		return nil, fmt.Errorf("unknown permissionPreset %q (want %s or %s)", preset, PresetReadOnly, PresetReadWrite)
	}
}
