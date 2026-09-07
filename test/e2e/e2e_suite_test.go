//go:build e2e
// +build e2e

/*
Copyright 2026 Thalassa Cloud.

Package e2e runs Kind + live Thalassa API smoke tests for Bucket and BucketAccess.

Requires:
  E2E_THALASSA_ENABLED=true
  E2E_THALASSA_ORGANISATION=...
  E2E_THALASSA_TOKEN=... (or E2E_THALASSA_TOKEN_FILE)
Optional: E2E_THALASSA_URL, E2E_THALASSA_REGION, E2E_THALASSA_INSECURE

Run: make test-e2e
*/

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/thalassa-cloud/thalassa-objectstorage-manager/test/utils"
)

var (
	managerImage = "example.com/thalassa-objectstorage-manager:v0.0.1"
	thalassaCfg  ThalassaE2EConfig
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting thalassa-objectstorage-manager e2e suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	var err error
	thalassaCfg, err = LoadThalassaE2EConfig()
	Expect(err).NotTo(HaveOccurred())
	if !thalassaCfg.Enabled {
		Skip(fmt.Sprintf("Thalassa live API e2e disabled. Set %s=true and configure %s + %s (or %s).",
			EnvThalassaEnabled, EnvThalassaOrganisation, EnvThalassaToken, EnvThalassaTokenFile))
	}
	Expect(thalassaCfg.Validate()).NotTo(HaveOccurred())

	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("loading the manager image on Kind")
	Expect(utils.LoadImageToKindClusterWithName(managerImage)).NotTo(HaveOccurred())

	// Avoid leaving KIND env empty for helpers.
	if os.Getenv("KIND_CLUSTER") == "" {
		_ = os.Setenv("KIND_CLUSTER", "thalassa-objectstorage-manager-test-e2e")
	}
})
