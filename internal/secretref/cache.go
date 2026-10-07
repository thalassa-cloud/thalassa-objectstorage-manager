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
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ParseNamespaces parses a comma-separated list of Kubernetes namespace names.
// Empty entries are dropped, duplicates are removed, and the result is sorted.
func ParseNamespaces(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		ns := strings.TrimSpace(part)
		if ns == "" {
			continue
		}
		if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
			return nil, fmt.Errorf("invalid namespace %q: %s", ns, strings.Join(errs, "; "))
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}
	slices.Sort(out)
	return out, nil
}

// SecretCacheOptions returns manager cache options that restrict the Secret
// informer to the given namespaces. When allowAllNamespaces is true, or when
// no namespaces are listed, the Secret informer stays cluster-scoped (the
// caller must have cluster-wide Secret list/watch).
func SecretCacheOptions(allowAllNamespaces bool, namespaces []string) cache.Options {
	if allowAllNamespaces || len(namespaces) == 0 {
		return cache.Options{}
	}
	nsCfg := make(map[string]cache.Config, len(namespaces))
	for _, ns := range namespaces {
		nsCfg[ns] = cache.Config{}
	}
	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {Namespaces: nsCfg},
		},
	}
}
