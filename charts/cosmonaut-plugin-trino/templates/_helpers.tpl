{{/*
Expand the name of the chart.
*/}}
{{- define "cosmonaut-plugin-trino.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "cosmonaut-plugin-trino.fullname" -}}
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
Common labels
*/}}
{{- define "cosmonaut-plugin-trino.labels" -}}
helm.sh/chart: {{ include "cosmonaut-plugin-trino.name" . }}-{{ .Chart.Version }}
{{ include "cosmonaut-plugin-trino.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cosmonaut-plugin-trino.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cosmonaut-plugin-trino.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
