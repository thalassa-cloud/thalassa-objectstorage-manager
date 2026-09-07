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

package bucket

import (
	"fmt"
	"strings"

	"github.com/thalassa-cloud/client-go/objectstorage"
)

const (
	// AdoptableLabelKey must be present on a Thalassa bucket before adoption is allowed
	// when the BucketAdoption feature gate is enabled.
	AdoptableLabelKey = "objectstorage.controllers.thalassa.cloud/adoptable"
	// AdoptableLabelValue is the required value for AdoptableLabelKey.
	AdoptableLabelValue = "true"
)

// DefaultAdoptionRequiredLabels returns the labels a Thalassa bucket must carry to be adoptable.
func DefaultAdoptionRequiredLabels() map[string]string {
	return map[string]string{
		AdoptableLabelKey: AdoptableLabelValue,
	}
}

// ParseLabelPairs parses "k=v,k2=v2" into a map. Empty string yields DefaultAdoptionRequiredLabels.
func ParseLabelPairs(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultAdoptionRequiredLabels(), nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 || strings.TrimSpace(kv[0]) == "" {
			return nil, fmt.Errorf("invalid label pair %q: expected key=value", part)
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	if len(out) == 0 {
		return DefaultAdoptionRequiredLabels(), nil
	}
	return out, nil
}

// CheckAdoptionEligibility returns an error if the existing bucket may not be adopted.
func CheckAdoptionEligibility(existing *objectstorage.ObjectStorageBucket, adoptionEnabled bool, requiredLabels map[string]string) error {
	if existing == nil {
		return fmt.Errorf("bucket is nil")
	}
	if !adoptionEnabled {
		return fmt.Errorf("Thalassa bucket %q already exists; adoption is disabled (enable feature gate BucketAdoption and label the bucket for adoption)", existing.Name)
	}
	if len(requiredLabels) == 0 {
		requiredLabels = DefaultAdoptionRequiredLabels()
	}
	labels := existing.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	var missing []string
	for k, want := range requiredLabels {
		if got, ok := labels[k]; !ok || got != want {
			missing = append(missing, fmt.Sprintf("%s=%s", k, want))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("Thalassa bucket %q is missing required adoption label(s): %s", existing.Name, strings.Join(missing, ", "))
	}
	return nil
}
