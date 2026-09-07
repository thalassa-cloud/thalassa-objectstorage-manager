//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/thalassa-cloud/client-go/objectstorage"
	bucketaccesspkg "github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/bucketaccess"
)

func registerBucketAccessStackSpecs() {
	var (
		osClient       *objectstorage.Client
		bucketManifest string
		accessManifest string
		bucketCRName   = "e2e-bucket"
		accessCRName   = "e2e-access"
		secretName     = "e2e-bucket-credentials"
		thalassaBucket string
	)

	BeforeAll(func() {
		var err error
		osClient, err = NewE2EObjectStorageClient(thalassaCfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(EnsureNamespace(TenantNamespace)).NotTo(HaveOccurred())

		bucketManifest = RenderBucketYAML(bucketCRName, TenantNamespace, thalassaCfg.Region)
		accessManifest = RenderBucketAccessYAML(accessCRName, TenantNamespace, bucketCRName, secretName, bucketaccesspkg.PresetReadOnly)
	})

	AfterAll(func() {
		_ = KubectlDeleteManifest(accessManifest)
		_ = KubectlDeleteManifest(bucketManifest)
		DeleteNamespaceIgnoreNotFound(TenantNamespace)
		if thalassaBucket != "" && osClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_ = osClient.DeleteBucket(ctx, thalassaBucket)
		}
	})

	It("reconciles Bucket to Ready", func() {
		Expect(KubectlApplyString(bucketManifest)).NotTo(HaveOccurred())
		_ = WaitCRResourceID("bucket", bucketCRName, TenantNamespace, 10*time.Minute, 10*time.Second)
		WaitCRReady("bucket", bucketCRName, TenantNamespace, 10*time.Minute, 10*time.Second)
		thalassaBucket = WaitBucketName(bucketCRName, TenantNamespace, 2*time.Minute, 5*time.Second)

		Eventually(func(g Gomega) {
			g.Expect(AssertBucketReady(context.Background(), osClient, thalassaBucket)).NotTo(HaveOccurred())
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("reconciles BucketAccess and writes Secret", func() {
		Expect(KubectlApplyString(accessManifest)).NotTo(HaveOccurred())
		WaitCRReady("bucketaccess", accessCRName, TenantNamespace, 10*time.Minute, 10*time.Second)
		WaitSecretHasKeys(TenantNamespace, secretName,
			[]string{"accessKey", "secretKey", "endpoint", "bucket"},
			5*time.Minute, 5*time.Second)
	})

	It("rotates credentials when annotated", func() {
		beforeKey, err := Kubectl("get", "secret", secretName, "-n", TenantNamespace,
			"-o", "jsonpath={.data.accessKey}")
		Expect(err).NotTo(HaveOccurred())
		Expect(beforeKey).NotTo(BeEmpty())

		_, err = Kubectl("annotate", "bucketaccess", accessCRName, "-n", TenantNamespace,
			bucketaccesspkg.RotateCredentialsAnnotation+"=true", "--overwrite")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			afterKey, err := Kubectl("get", "secret", secretName, "-n", TenantNamespace,
				"-o", "jsonpath={.data.accessKey}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(afterKey).NotTo(BeEmpty())
			g.Expect(afterKey).NotTo(Equal(beforeKey))
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		WaitCRReady("bucketaccess", accessCRName, TenantNamespace, 2*time.Minute, 5*time.Second)
	})
}
