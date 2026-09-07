//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/thalassa-cloud/thalassa-objectstorage-manager/test/utils"
)

func Kubectl(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	return utils.Run(cmd)
}

func KubectlWithStdin(stdin string, args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(stdin)
	return utils.Run(cmd)
}

func KubectlApplyString(manifest string) error {
	_, err := KubectlWithStdin(manifest, "apply", "-f", "-")
	return err
}

func KubectlDeleteManifest(manifest string) error {
	_, err := KubectlWithStdin(manifest, "delete", "--ignore-not-found", "-f", "-")
	return err
}

func WaitDeploymentRollout(timeout time.Duration) error {
	_, err := Kubectl("rollout", "status", "deployment/"+DeploymentName, "-n", Namespace,
		fmt.Sprintf("--timeout=%s", timeout.String()))
	return err
}

func GetJSONPath(resource, name, ns, jsonpath string) (string, error) {
	return Kubectl("get", resource, name, "-n", ns, "-o", "jsonpath="+jsonpath)
}

func EnsureNamespace(ns string) error {
	if _, err := Kubectl("get", "ns", ns); err == nil {
		return nil
	}
	_, err := Kubectl("create", "ns", ns)
	return err
}

func DeleteNamespaceIgnoreNotFound(ns string) {
	_, _ = Kubectl("delete", "ns", ns, "--ignore-not-found", "--wait=false")
}

func ReadTokenFromConfig(cfg ThalassaE2EConfig) ([]byte, error) {
	if cfg.Token != "" {
		return []byte(cfg.Token), nil
	}
	if cfg.TokenFile == "" {
		return nil, fmt.Errorf("no token or token file")
	}
	b, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(string(b))), nil
}
