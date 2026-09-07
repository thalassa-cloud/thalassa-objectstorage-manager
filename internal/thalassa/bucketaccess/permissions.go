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
)

// ReadOnlyPermissions is the least-privilege object read + list set.
var ReadOnlyPermissions = []string{
	"s3:GetObject",
	"s3:GetObjectVersion",
	"s3:ListBucket",
	"s3:ListBucketVersions",
	"s3:GetObjectTagging",
	"s3:GetObjectVersionTagging",
	"s3:GetBucketVersioning",
}

// ReadWritePermissions is the default grant set (objects + list + tagging + multipart).
var ReadWritePermissions = []string{
	"s3:GetObject",
	"s3:GetObjectVersion",
	"s3:PutObject",
	"s3:DeleteObject",
	"s3:DeleteObjectVersion",
	"s3:ListMultipartUploadParts",
	"s3:AbortMultipartUpload",
	"s3:ListBucket",
	"s3:ListBucketVersions",
	"s3:ListBucketMultipartUploads",
	"s3:GetBucketAcl",
	"s3:GetBucketVersioning",
	"s3:GetLifecycleConfiguration",
	"s3:GetObjectTagging",
	"s3:PutObjectTagging",
	"s3:DeleteObjectTagging",
	"s3:GetObjectVersionTagging",
	"s3:PutObjectVersionTagging",
	"s3:DeleteObjectVersionTagging",
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
		"s3:GetObjectVersionTagging",
		"s3:GetBucketLocation",
		"s3:GetBucketPolicyStatus",
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
