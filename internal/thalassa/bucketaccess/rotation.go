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
	"strings"
)

// RotateCredentialsAnnotation, when set to a truthy value ("true"/"1"), forces
// recreation of the Thalassa access credential and Secret material on the next reconcile.
// The controller clears the annotation after a successful rotation.
const RotateCredentialsAnnotation = "objectstorage.controllers.thalassa.cloud/rotate-credentials"

// WantsCredentialRotation reports whether the annotation requests a one-shot rotation.
func WantsCredentialRotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	v := annotations[RotateCredentialsAnnotation]
	return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
}
