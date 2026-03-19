{{/*
Expand the name of the chart.
*/}}
{{- define "sfs-iaas.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "sfs-iaas.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "sfs-iaas.labels" -}}
helm.sh/chart: {{ include "sfs-iaas.chart" . }}
{{ include "sfs-iaas.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.Version | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.global.labels }}
{{ . | toYaml }}
{{- end }}
superphenix.net/gitops: {{ .Values.gitops | quote }}
superphenix.net/organizationID: spx-{{ .Values.organizationID }}
superphenix.net/organizationName: {{ .Values.organizationName | quote }}
superphenix.net/projectID: spx-{{ .Values.projectID }}
superphenix.net/projectName: {{ .Values.projectName | quote }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "sfs-iaas.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sfs-iaas.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Returns the SPX effective ID of a resource
*/}}
{{- define "sfs-iaas.spxEID" -}}
{{ printf "spx-%s" (include "sfs-iaas.getUUIDv5" (dict "NS" .projectID "NAME" .localID)) }}
{{- end }}

{{/*
Generate UUIDv5 through external templating
*/}}
{{- define "sfs-iaas.getUUIDv5" -}}
{{- printf "<spx-uuidv5 %s %s>" .NS .NAME }}
{{- end }}

{{/*
Returns the replication strategy for a project
*/}}
{{- define "sfs-iaas.replicationStrategy" -}}
{{- with .Values.replication }}

{{- $replicationEnabled := true }}
{{- $class := .schedule }}

{{- range $az, $strategy := .availabilityZones }}
{{- if eq $az $.Values.location }}

{{- $replicationEnabled = $strategy.enabled }}
{{- with $strategy.schedule }}
{{- $class = . }}
{{- end }}

{{- end }}
{{- end }}

{{- if and $replicationEnabled $class }}
replication.superphenix.net/classSelector: {{ $class | quote }}
{{- end }}

{{- end }}
{{- end }}
