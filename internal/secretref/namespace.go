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
	"fmt"
)

// ErrCrossNamespaceForbidden is returned when a secret reference targets another
// namespace while cross-namespace secret refs are disabled on the manager.
var ErrCrossNamespaceForbidden = errors.New("cross-namespace secret reference is not allowed")

// Resolve returns the namespace to use for a secret reference.
// An empty refNamespace defaults to resourceNamespace.
// When allowAllNamespaces is false, a non-empty refNamespace that differs from
// resourceNamespace is rejected (confused-deputy prevention).
func Resolve(resourceNamespace, refNamespace string, allowAllNamespaces bool) (string, error) {
	if refNamespace == "" || refNamespace == resourceNamespace {
		return resourceNamespace, nil
	}
	if !allowAllNamespaces {
		return "", fmt.Errorf("%w: referenced namespace %q differs from resource namespace %q (enable --allow-all-namespaces-secret-ref to permit)",
			ErrCrossNamespaceForbidden, refNamespace, resourceNamespace)
	}
	return refNamespace, nil
}
