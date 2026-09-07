# Helm chart for deploying the Thalassa Cloud Object Storage Manager

## Authentication

Configure `thalassa.enabled: true` and one of:

- `authMethod: tokenExchange` (recommended) — federated workload identity
- `authMethod: pat` — personal access token from a mounted Secret
- `authMethod: oidcClientCredentials` — OAuth2 client credentials

### Token exchange example

```yaml
thalassa:
  enabled: true
  url: "https://api.thalassa.cloud/"
  organisation: "<ORGANISATION_ID>"
  authMethod: tokenExchange
  tokenExchange:
    serviceAccountId: "<THALASSA_SERVICE_ACCOUNT_ID>"
    projectedToken:
      enabled: true
      audience: "https://api.thalassa.cloud"

defaultRegion: nl-01
```

Set `defaultRegion` (or `thalassa.region`) so Bucket resources can omit `spec.region`.

## Security

### Secret RBAC

**Default (recommended):** namespaced Secret access only.

```bash
# Tenant namespaces that host BucketAccess Secrets (release NS is always included)
--set rbac.secretNamespaces={default,apps}
```

**Cluster-scoped (security-sensitive):** allow Secrets in any namespace.

```bash
--set rbac.clusterScopedSecrets=true
```

This enables `--allow-all-namespaces-secret-ref` and grants cluster-wide Secret RBAC. Prefer `secretNamespaces` unless you deliberately need multi-namespace Secret writes. Anyone who can create `BucketAccess` can then use the controller as a confused deputy to write Secrets elsewhere.

`allowAllNamespacesSecretRef` remains supported as a deprecated alias for `rbac.clusterScopedSecrets`.

### Bucket adoption (`featureGates.BucketAdoption`)

**Default: off.** Existing Thalassa buckets are not adopted by name.

When enabled, adoption requires the remote bucket to carry labels from `bucketAdoption.requiredLabels` (default `objectstorage.controllers.thalassa.cloud/adoptable=true`).
