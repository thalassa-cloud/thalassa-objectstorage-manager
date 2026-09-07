//go:build e2e
// +build e2e

package e2e

const (
	Namespace = "thalassa-objectstorage-manager-system"

	DeploymentName = "thalassa-objectstorage-manager-controller-manager"

	ServiceAccountName = "thalassa-objectstorage-manager-controller-manager"

	ThalassaTokenSecretName = "e2e-thalassa-manager-credentials"
	ThalassaTokenSecretKey  = "token"
	ThalassaTokenMountPath  = "/var/run/thalassa-e2e"

	thalassaTokenFilePath = ThalassaTokenMountPath + "/" + ThalassaTokenSecretKey

	// TenantNamespace hosts sample Bucket / BucketAccess CRs for the smoke stack.
	TenantNamespace = "e2e-objectstorage"
)
