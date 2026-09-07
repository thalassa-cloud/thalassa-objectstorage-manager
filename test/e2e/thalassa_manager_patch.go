//go:build e2e
// +build e2e

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

// ApplyThalassaManagerIntegration creates the token Secret and patches the manager Deployment.
func ApplyThalassaManagerIntegration(cfg ThalassaE2EConfig) error {
	tokenBytes, err := ReadTokenFromConfig(cfg)
	if err != nil {
		return err
	}
	if err := applyCredentialsSecret(tokenBytes); err != nil {
		return fmt.Errorf("credentials secret: %w", err)
	}
	if err := patchManagerThalassaFlags(cfg); err != nil {
		return fmt.Errorf("patch deployment: %w", err)
	}
	return nil
}

func applyCredentialsSecret(token []byte) error {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
  %s: %s
`, ThalassaTokenSecretName, Namespace, ThalassaTokenSecretKey, base64.StdEncoding.EncodeToString(token))
	_, err := KubectlWithStdin(manifest, "apply", "-f", "-")
	return err
}

func patchManagerThalassaFlags(cfg ThalassaE2EConfig) error {
	args := []string{
		"--leader-elect",
		"--health-probe-bind-address=:8081",
		"--organisation=" + cfg.Organisation,
		"--thalassa-url=" + cfg.ThalassaURL,
		"--thalassa-token-file=" + thalassaTokenFilePath,
		"--default-region=" + cfg.Region,
	}
	if cfg.Insecure {
		args = append(args, "--thalassa-insecure=true")
	}

	patch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/spec/template/spec/containers/0/args",
			"value": args,
		},
		{
			"op":   "add",
			"path": "/spec/template/spec/containers/0/volumeMounts/-",
			"value": map[string]interface{}{
				"name":      "thalassa-e2e-token",
				"mountPath": ThalassaTokenMountPath,
				"readOnly":  true,
			},
		},
		{
			"op":   "add",
			"path": "/spec/template/spec/volumes/-",
			"value": map[string]interface{}{
				"name": "thalassa-e2e-token",
				"secret": map[string]interface{}{
					"secretName": ThalassaTokenSecretName,
				},
			},
		},
	}

	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "e2e-manager-patch-*.json")
	if err != nil {
		return err
	}
	path := tmp.Name()
	_, _ = tmp.Write(raw)
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	_, err = Kubectl("patch", "deployment", DeploymentName, "-n", Namespace, "--type=json", "--patch-file="+path)
	if err != nil {
		return fmt.Errorf("%w (hint: stale cluster — run make undeploy && make deploy)", err)
	}
	return nil
}
