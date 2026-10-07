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

package managerhealth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

const cacheSyncTimeout = 2 * time.Second

// CacheSyncCheck fails readiness while manager informers have not synced
// (for example when a namespaced Secret Role is paired with a cluster-scoped informer).
func CacheSyncCheck(synced func(context.Context) bool) healthz.Checker {
	return func(req *http.Request) error {
		if synced == nil {
			return errors.New("cache sync checker is not configured")
		}
		ctx := context.Background()
		if req != nil {
			ctx = req.Context()
		}
		ctx, cancel := context.WithTimeout(ctx, cacheSyncTimeout)
		defer cancel()
		if !synced(ctx) {
			return errors.New("cache not synced")
		}
		return nil
	}
}
