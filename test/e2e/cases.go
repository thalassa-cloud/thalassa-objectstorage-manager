//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"time"
)

func RenderBucketYAML(name, ns, region string) string {
	bucketName := fmt.Sprintf("e2e-%s-%d", name, time.Now().Unix()%100000)
	return fmt.Sprintf(`apiVersion: objectstorage.controllers.thalassa.cloud/v1
kind: Bucket
metadata:
  name: %s
  namespace: %s
spec:
  name: %s
  region: %q
  public: false
  versioning: Disabled
`, name, ns, bucketName, region)
}

func RenderBucketAccessYAML(name, ns, bucketRef, secretName, preset string) string {
	return fmt.Sprintf(`apiVersion: objectstorage.controllers.thalassa.cloud/v1
kind: BucketAccess
metadata:
  name: %s
  namespace: %s
spec:
  bucketRefs:
    - name: %s
  writeSecretToRef:
    name: %s
  permissionPreset: %s
  description: e2e bucket access
`, name, ns, bucketRef, secretName, preset)
}
