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
  organisation: "<ORGANISATION_SLUG_OR_ID>"
  authMethod: tokenExchange
  tokenExchange:
    serviceAccountId: "<THALASSA_SERVICE_ACCOUNT_ID>"
    projectedToken:
      enabled: true
      audience: "https://api.thalassa.cloud"

defaultRegion: nl-01

# DELETE-only validating webhook for Bucket deleteProtection (requires cert-manager by default)
webhook:
  enabled: true
  certManager:
    enabled: true
```

Set `defaultRegion` (or `thalassa.region`) so Bucket resources can omit `spec.region`.

## Bucket delete protection

`Bucket.spec.deleteProtection: true` blocks `DELETE` until you set it back to `false`. Enforcement uses a validating webhook (`webhook.enabled`, default true).

- Default: cert-manager issues the webhook TLS cert (`webhook.certManager.enabled: true`). Install [cert-manager](https://cert-manager.io/) in the cluster first.
- Without cert-manager: set `webhook.certManager.enabled=false` and `webhook.tls.secretName` to an existing TLS Secret (`tls.crt` / `tls.key`) whose SANs include the webhook Service DNS names.
- To disable the webhook entirely: `--set webhook.enabled=false` (passes `--enable-webhooks=false`; delete protection will not be enforced).
- Local `make run` / IDE: pass `--enable-webhooks=false` or set `ENABLE_WEBHOOKS=false` when you do not have webhook TLS or a ValidatingWebhookConfiguration.

## Security

### Secret RBAC

**Default (recommended):** namespaced Secret access only.

```bash
# Tenant namespaces that host BucketAccess Secrets (release NS is always included)
--set rbac.secretNamespaces={default,apps}
```

The chart grants namespaced Secret `Role`s and passes those namespaces as `--secret-namespaces` so the Secret informer is not cluster-scoped. Without that, cache sync never completes and no `Bucket` / `BucketAccess` is reconciled.

**Cluster-scoped (security-sensitive):** allow Secrets in any namespace.

```bash
--set rbac.clusterScopedSecrets=true
```

This enables `--allow-all-namespaces-secret-ref` and grants cluster-wide Secret RBAC. Prefer `secretNamespaces` unless you deliberately need multi-namespace Secret writes. Anyone who can create `BucketAccess` can then use the controller as a confused deputy to write Secrets elsewhere.

`allowAllNamespacesSecretRef` remains supported as a deprecated alias for `rbac.clusterScopedSecrets`.

### Bucket adoption (`featureGates.BucketAdoption`)

**Default: off.** Existing Thalassa buckets are not adopted by name.

When enabled, adoption requires the remote bucket to carry labels from `bucketAdoption.requiredLabels` (default `objectstorage.controllers.thalassa.cloud/adoptable=true`).
