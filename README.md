# Thalassa Cloud Object Storage Manager

Provision Thalassa Cloud Object Storage buckets and in-cluster access credentials. This manager is an extension for Thalassa Cloud Kubernetes clusters: declare `Bucket` and `BucketAccess` resources and have them reconciled with the Thalassa Object Storage and IAM APIs.

> **Preview**
> Initial release. The API may change.

## Custom resources

| Kind           | API group                                     | Purpose                                                                                                                                                                  |
| -------------- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Bucket`       | `objectstorage.controllers.thalassa.cloud/v1` | Creates/updates/deletes a Thalassa object storage bucket                                                                                                                 |
| `BucketAccess` | `objectstorage.controllers.thalassa.cloud/v1` | Managed mode: IAM service account + `objectStorage` credential + Secret. External mode (`principalRef`): grant an existing User or ServiceAccount via bucket policy only |

**BucketAccess modes** (exactly one of `writeSecretToRef` or `principalRef`):

- **Managed** — `writeSecretToRef`: create SA + credentials, grant policy, write Secret keys `accessKey`, `secretKey`, `endpoint`, `bucket`, `buckets`
- **External** — `principalRef.kind` (`User` \| `ServiceAccount`) + `identity`: policy grant only

`permissionPreset` defaults to **ReadWrite** when omitted (use `ReadOnly` for least privilege). Admin actions (`s3:*`, `PutBucketPolicy`, `DeleteBucket`, …) are rejected.

By default `Bucket.spec.generateNameSuffix` is **true**, so the Thalassa bucket name is `<base>-<random>` (see `status.bucketName`). Set `generateNameSuffix: false` for a fixed name.

## Prerequisites

- Go 1.26+
- kubectl
- A Kubernetes cluster with Thalassa API access

## Deploy with Helm and workload identity federation

### 1. Bootstrap federated identity

```bash
export ORGANISATION_ID="<your-org-id>"
export CLUSTER_ID="<your-cluster-id>"

tcloud iam workload-identity-federation bootstrap kubernetes \
  --cluster "$CLUSTER_ID" \
  --namespace thalassa-objectstorage-manager \
  --service-account thalassa-objectstorage-manager \
  --role thalassa:kubernetes:ObjectStorageManager
```

> Note: the role `thalassa:kubernetes:ObjectStorageManager` may not yet be available for your organisation. Permissions this manager needs:
>
> - object_storage_bucket: create, read, update, delete, list
> - service_accounts: create, read, update, delete, list
> - service_account_access_credentials: create, read, update, delete, list
> - organisation memberships: read (required for external `principalRef.kind: User`)

Copy the Thalassa service account ID from the output into `THALASSA_SERVICE_ACCOUNT_ID`.

### 2. Install CRDs and controller

```bash
export ORGANISATION_ID="<ORGANISATION_ID>"
export THALASSA_SERVICE_ACCOUNT_ID="<THALASSA_SERVICE_ACCOUNT_ID>"

helm upgrade --install thalassa-objectstorage-manager-crds \
  oci://ghcr.io/thalassa-cloud/charts/thalassa-objectstorage-manager-crds:<version> \
  --namespace thalassa-objectstorage-manager \
  --create-namespace

helm upgrade --install thalassa-objectstorage-manager \
  oci://ghcr.io/thalassa-cloud/charts/thalassa-objectstorage-manager:<version> \
  --namespace thalassa-objectstorage-manager \
  --create-namespace \
  --set thalassa.organisation="$ORGANISATION_ID" \
  --set thalassa.tokenExchange.serviceAccountId="$THALASSA_SERVICE_ACCOUNT_ID" \
  --set defaultRegion=nl-01 \
  --set rbac.secretNamespaces={default} \
  --set enableServiceMonitor=false
```

`rbac.secretNamespaces` must list every namespace where `BucketAccess` writes Secrets (the release namespace is always included). For the hack sample use `--set rbac.secretNamespaces={objectstorage-hack}` (or include both). For cluster-wide Secret access (security-sensitive): `--set rbac.clusterScopedSecrets=true`.

Other auth methods (personal access token, OAuth2 client credentials) are documented in [`chart/thalassa-objectstorage-manager/README.md`](chart/thalassa-objectstorage-manager/README.md).

## Example

```yaml
apiVersion: objectstorage.controllers.thalassa.cloud/v1
kind: Bucket
metadata:
  name: app-data
  namespace: default
spec:
  name: app-data
  generateNameSuffix: false # omit or true → app-data-<random>; see status.bucketName
  region: nl-01
---
apiVersion: objectstorage.controllers.thalassa.cloud/v1
kind: BucketAccess
metadata:
  name: app-data-access
  namespace: default
spec:
  bucketRefs:
    - name: app-data
  writeSecretToRef:
    name: app-data-credentials
  # permissionPreset defaults to ReadWrite
```

## Security notes

- Connection credentials are written only to Kubernetes Secrets.
- Secret writes are same-namespace by default. Prefer listing tenant namespaces with Helm `--set rbac.secretNamespaces={ns1,ns2}` (release namespace is always included). Only use `--set rbac.clusterScopedSecrets=true` (enables `--allow-all-namespaces-secret-ref` + cluster-wide Secret RBAC) when you intentionally need cross-namespace Secret writes.
- Existing Secrets are never overwritten unless already owned by the `BucketAccess` (ownerRef or ownership label).
- Bucket adoption is behind `--feature-gates=BucketAdoption=true` and requires labels on the Thalassa bucket (`objectstorage.controllers.thalassa.cloud/adoptable=true` by default).
- `permissionPreset` defaults to `ReadWrite`; prefer `ReadOnly` when write access is not needed. Admin actions (`s3:*`, `PutBucketPolicy`, `DeleteBucket`, …) are rejected.
- Rotate credentials: `kubectl annotate bucketaccess <name> objectstorage.controllers.thalassa.cloud/rotate-credentials=true`

## E2E (Kind + live Thalassa API)

```bash
export E2E_THALASSA_ENABLED=true
export E2E_THALASSA_ORGANISATION=<org-id>
export E2E_THALASSA_TOKEN=<pat>   # or E2E_THALASSA_TOKEN_FILE
# optional: E2E_THALASSA_URL, E2E_THALASSA_REGION (default nl-01), E2E_THALASSA_INSECURE=true

make test-e2e
```

`make test-e2e` always creates and deletes a Kind cluster. Without `E2E_THALASSA_ENABLED=true` the suite still boots Kind, then skips the live Thalassa cases before building/pushing the manager image.

## Hack sample (bucket + mc permission check)

With the manager running and CRDs installed (and Secret RBAC covering `objectstorage-hack` if using Helm):

```bash
kubectl apply -k hack/samples
kubectl -n objectstorage-hack get bucket,bucketaccess,secret,deploy
kubectl -n objectstorage-hack logs deploy/hack-demo-mc-rw -f
kubectl -n objectstorage-hack logs deploy/hack-demo-mc-ro -f
```

Adjust `spec.region` / `spec.name` in `hack/samples/bucket.yaml` if needed. Tear down with `kubectl delete -k hack/samples`.

## Development

```bash
make generate manifests
make test
make run   # uses current kubeconfig; requires Thalassa flags or .env
```

Local debug: copy `.env.example` → `.env`, set organisation + auth, and optionally `CLUSTER_ID` for the kubeconfig preLaunchTask.

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
