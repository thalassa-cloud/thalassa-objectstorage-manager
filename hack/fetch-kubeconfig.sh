#!/usr/bin/env bash
# Fetch a kubeconfig for local manager debugging via tcloud.
#
# Requires CLUSTER_ID in the environment or project .env.
# Auth uses the current tcloud context, or THALASSA_* from .env when set.
#
# Usage:
#   ./hack/fetch-kubeconfig.sh
#   ./hack/fetch-kubeconfig.sh <cluster-id>
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${KUBECONFIG_OUT:-${ROOT}/hack/cluster.kubeconfig}"
ENV_FILE="${ENV_FILE:-${ROOT}/.env}"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi

CLUSTER_ID="${1:-${CLUSTER_ID:-}}"
if [[ -z "${CLUSTER_ID}" ]]; then
  echo "CLUSTER_ID is required (arg or .env). Example:" >&2
  echo "  CLUSTER_ID=<cluster-id> ./hack/fetch-kubeconfig.sh" >&2
  echo "  tcloud kubernetes kubeconfig <cluster-id>" >&2
  exit 1
fi

if ! command -v tcloud >/dev/null 2>&1; then
  echo "tcloud CLI not found on PATH" >&2
  exit 1
fi

args=(kubernetes kubeconfig "${CLUSTER_ID}")

if [[ -n "${THALASSA_ORGANISATION:-}" ]]; then
  args+=(--organisation "${THALASSA_ORGANISATION}")
fi
if [[ -n "${THALASSA_URL:-}" ]]; then
  args+=(--api "${THALASSA_URL}")
fi
if [[ -n "${THALASSA_CLIENT_ID:-}" ]]; then
  args+=(--client-id "${THALASSA_CLIENT_ID}")
fi
if [[ -n "${THALASSA_CLIENT_SECRET:-}" ]]; then
  args+=(--client-secret "${THALASSA_CLIENT_SECRET}")
fi
if [[ -n "${THALASSA_PROJECT:-}" ]]; then
  args+=(--project "${THALASSA_PROJECT}")
fi
# Prefer PAT from .env only when client credentials are not set.
if [[ -z "${THALASSA_CLIENT_ID:-}" && -n "${THALASSA_TOKEN:-}" ]]; then
  args+=(--token "${THALASSA_TOKEN}")
fi

umask 077
mkdir -p "$(dirname "${OUT}")"
tmp="$(mktemp "${OUT}.XXXXXX")"
cleanup() { rm -f "${tmp}"; }
trap cleanup EXIT

echo "fetching kubeconfig for cluster ${CLUSTER_ID} -> ${OUT}"
tcloud "${args[@]}" >"${tmp}"
mv "${tmp}" "${OUT}"
trap - EXIT
chmod 600 "${OUT}"
echo "wrote ${OUT}"
