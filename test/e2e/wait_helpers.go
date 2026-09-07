//go:build e2e
// +build e2e

package e2e

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	readyConditionJSONPath = `{.status.conditions[?(@.type=="Ready")].status}`
	resourceIDJSONPath     = `{.status.resourceId}`
	bucketNameJSONPath     = `{.status.bucketName}`
)

func WaitCRResourceID(resource, name, ns string, timeout, interval time.Duration) string {
	GinkgoHelper()
	var id string
	Eventually(func(g Gomega) {
		out, err := GetJSONPath(resource, name, ns, resourceIDJSONPath)
		g.Expect(err).NotTo(HaveOccurred())
		id = strings.TrimSpace(out)
		g.Expect(id).NotTo(BeEmpty())
	}, timeout, interval).Should(Succeed())
	return id
}

func WaitCRReady(resource, name, ns string, timeout, interval time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		out, err := GetJSONPath(resource, name, ns, readyConditionJSONPath)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(out)).To(Equal("True"))
	}, timeout, interval).Should(Succeed())
}

func WaitBucketName(name, ns string, timeout, interval time.Duration) string {
	GinkgoHelper()
	var bucket string
	Eventually(func(g Gomega) {
		out, err := GetJSONPath("bucket", name, ns, bucketNameJSONPath)
		g.Expect(err).NotTo(HaveOccurred())
		bucket = strings.TrimSpace(out)
		g.Expect(bucket).NotTo(BeEmpty())
	}, timeout, interval).Should(Succeed())
	return bucket
}

func WaitSecretHasKeys(ns, secretName string, keys []string, timeout, interval time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, key := range keys {
			out, err := Kubectl("get", "secret", secretName, "-n", ns,
				"-o", "jsonpath={.data."+key+"}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(), "secret key %s in %s/%s", key, ns, secretName)
		}
	}, timeout, interval).Should(Succeed())
}
