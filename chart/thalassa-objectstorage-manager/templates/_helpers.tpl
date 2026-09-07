{{/*
Expand the name of the chart.
*/}}
{{- define "objectstorageManager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "objectstorageManager.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "objectstorageManager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}

{{- define "objectstorageManager.baseLabels" -}}
helm.sh/chart: {{ include "objectstorageManager.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "objectstorageManager.labels" -}}
{{ include "objectstorageManager.baseLabels" . }}
{{ include "objectstorageManager.selectorLabels" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "objectstorageManager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "objectstorageManager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: operator
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "objectstorageManager.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "objectstorageManager.name" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Labels for optional end-user ClusterRoles (thalassa:objectstorage:reader, etc.) — not the operator workload.
*/}}
{{- define "objectstorageManager.userClusterRoleLabels" -}}
{{ include "objectstorageManager.baseLabels" . }}
app.kubernetes.io/name: {{ include "objectstorageManager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: user-rbac
{{- end }}

{{/*
True when cluster-wide Secret access is enabled (rbac.clusterScopedSecrets or legacy allowAllNamespacesSecretRef).
*/}}
{{- define "objectstorageManager.clusterScopedSecrets" -}}
{{- if or .Values.rbac.clusterScopedSecrets .Values.allowAllNamespacesSecretRef -}}
true
{{- else -}}
false
{{- end -}}
{{- end }}
