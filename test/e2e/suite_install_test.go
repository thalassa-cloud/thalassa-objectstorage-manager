//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/thalassa-cloud/thalassa-objectstorage-manager/test/utils"
)

var _ = Describe("thalassa-objectstorage-manager", Ordered, func() {
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", Namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("labeling the namespace for restricted PSA")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", Namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("wiring Thalassa credentials into the manager")
		Expect(ApplyThalassaManagerIntegration(thalassaCfg)).NotTo(HaveOccurred())

		By("waiting for manager rollout")
		Expect(WaitDeploymentRollout(5 * time.Minute)).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("undeploying controller")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)
		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
		By("deleting manager namespace")
		DeleteNamespaceIgnoreNotFound(Namespace)
	})

	Context("Bucket + BucketAccess smoke", Label("stack"), Ordered, func() {
		registerBucketAccessStackSpecs()
	})
})
