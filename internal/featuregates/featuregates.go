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

package featuregates

import (
	"fmt"
	"strconv"
	"strings"
)

// Known feature gates.
const (
	// BucketAdoption allows adopting an existing Thalassa bucket by name when
	// the remote bucket carries the required adoption labels.
	BucketAdoption = "BucketAdoption"
)

// Default returns the default enabled state for known gates (all off).
func Default() map[string]bool {
	return map[string]bool{
		BucketAdoption: false,
	}
}

// Parse parses a comma-separated feature-gates string of the form
// "GateA=true,GateB=false". Unknown gates are rejected.
func Parse(raw string) (map[string]bool, error) {
	out := Default()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid feature gate %q: expected Name=true|false", part)
		}
		name := strings.TrimSpace(kv[0])
		if _, ok := out[name]; !ok {
			return nil, fmt.Errorf("unknown feature gate %q", name)
		}
		val, err := strconv.ParseBool(strings.TrimSpace(kv[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid value for feature gate %q: %w", name, err)
		}
		out[name] = val
	}
	return out, nil
}

// Enabled reports whether the named gate is enabled.
func Enabled(gates map[string]bool, name string) bool {
	if gates == nil {
		return false
	}
	return gates[name]
}
