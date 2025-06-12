{{/*
Expand the name of the chart.
*/}}
{{- define "roost-keeper.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Configuration checksum for rolling updates
*/}}
{{- define "roost-keeper.configChecksum" -}}
{{- $config := dict "values" .Values "chart" .Chart "release" .Release -}}
{{- $config | toYaml | sha256sum -}}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "roost-keeper.fullname" -}}
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
{{- define "roost-keeper.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "roost-keeper.labels" -}}
helm.sh/chart: {{ include "roost-keeper.chart" . }}
{{ include "roost-keeper.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.extra.labels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "roost-keeper.selectorLabels" -}}
app.kubernetes.io/name: {{ include "roost-keeper.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Controller labels
*/}}
{{- define "roost-keeper.controllerLabels" -}}
{{ include "roost-keeper.labels" . }}
app.kubernetes.io/component: controller
control-plane: controller-manager
{{- end }}

{{/*
Controller selector labels
*/}}
{{- define "roost-keeper.controllerSelectorLabels" -}}
{{ include "roost-keeper.selectorLabels" . }}
app.kubernetes.io/component: controller
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "roost-keeper.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (printf "%s-controller-manager" (include "roost-keeper.fullname" .)) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "roost-keeper.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repository := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Create the namespace name
*/}}
{{- define "roost-keeper.namespace" -}}
{{- default .Release.Namespace .Values.global.namespace }}
{{- end }}

{{/*
Environment-specific values merge helper
*/}}
{{- define "roost-keeper.environmentValues" -}}
{{- $environment := .Values.environment | default "production" }}
{{- $envConfig := index .Values.environments $environment }}
{{- if $envConfig }}
{{- $merged := mergeOverwrite .Values $envConfig }}
{{- toYaml $merged }}
{{- else }}
{{- toYaml .Values }}
{{- end }}
{{- end }}

{{/*
Create image pull secrets
*/}}
{{- define "roost-keeper.imagePullSecrets" -}}
{{- $secrets := list }}
{{- if .Values.global.imagePullSecrets }}
{{- $secrets = concat $secrets .Values.global.imagePullSecrets }}
{{- end }}
{{- if .Values.image.pullSecrets }}
{{- $secrets = concat $secrets .Values.image.pullSecrets }}
{{- end }}
{{- if $secrets }}
imagePullSecrets:
{{- range $secrets }}
- name: {{ . }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create resource requirements
*/}}
{{- define "roost-keeper.resources" -}}
{{- with .Values.controller.resources }}
resources:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create affinity rules
*/}}
{{- define "roost-keeper.affinity" -}}
{{- with .Values.controller.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create node selector
*/}}
{{- define "roost-keeper.nodeSelector" -}}
{{- with .Values.controller.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create tolerations
*/}}
{{- define "roost-keeper.tolerations" -}}
{{- with .Values.controller.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create topology spread constraints
*/}}
{{- define "roost-keeper.topologySpreadConstraints" -}}
{{- if .Values.highAvailability.topologySpreadConstraints.enabled }}
topologySpreadConstraints:
- maxSkew: {{ .Values.highAvailability.topologySpreadConstraints.maxSkew }}
  topologyKey: {{ .Values.highAvailability.topologySpreadConstraints.topologyKey }}
  whenUnsatisfiable: {{ .Values.highAvailability.topologySpreadConstraints.whenUnsatisfiable }}
  labelSelector:
    matchLabels:
      {{- include "roost-keeper.controllerSelectorLabels" . | nindent 6 }}
{{- with .Values.highAvailability.topologySpreadConstraints.additional }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create security context
*/}}
{{- define "roost-keeper.securityContext" -}}
{{- with .Values.controller.securityContext }}
securityContext:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create container security context
*/}}
{{- define "roost-keeper.containerSecurityContext" -}}
{{- with .Values.controller.containerSecurityContext }}
securityContext:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Create controller command arguments
*/}}
{{- define "roost-keeper.controllerArgs" -}}
{{- $args := list }}
{{- if .Values.controller.leaderElection.enabled }}
{{- $args = append $args "--leader-elect=true" }}
{{- $args = append $args (printf "--leader-election-id=%s" .Values.controller.leaderElection.id) }}
{{- $args = append $args (printf "--leader-election-namespace=%s" (include "roost-keeper.namespace" .)) }}
{{- $args = append $args (printf "--leader-election-lease-duration=%s" .Values.controller.leaderElection.leaseDuration) }}
{{- $args = append $args (printf "--leader-election-renew-deadline=%s" .Values.controller.leaderElection.renewDeadline) }}
{{- $args = append $args (printf "--leader-election-retry-period=%s" .Values.controller.leaderElection.retryPeriod) }}
{{- else }}
{{- $args = append $args "--leader-elect=false" }}
{{- end }}
{{- $args = append $args "--health-probe-bind-address=:8081" }}
{{- $args = append $args "--metrics-bind-address=:8080" }}
{{- if .Values.performance.enabled }}
{{- $args = append $args (printf "--max-concurrent-reconciles=%d" (.Values.performance.maxConcurrentReconciles | int)) }}
{{- $args = append $args (printf "--cache-sync-period=%s" .Values.performance.cacheSyncPeriod) }}
{{- end }}
{{- if .Values.webhook.enabled }}
{{- $args = append $args (printf "--webhook-port=%d" (.Values.webhook.port | int)) }}
{{- $args = append $args "--webhook-cert-dir=/tmp/k8s-webhook-server/serving-certs" }}
{{- end }}
{{- $args = append $args (printf "--zap-log-level=%s" .Values.observability.logging.level) }}
{{- if eq .Values.observability.logging.format "json" }}
{{- $args = append $args "--zap-encoder=json" }}
{{- end }}
{{- if .Values.observability.logging.development }}
{{- $args = append $args "--zap-development=true" }}
{{- end }}
{{- $args = append $args "--metrics-secure=true" }}
{{- $args = append $args "--enable-http2=false" }}
{{- range .Values.controller.extraArgs }}
{{- $args = append $args . }}
{{- end }}
{{- toYaml $args }}
{{- end }}

{{/*
Create environment variables
*/}}
{{- define "roost-keeper.env" -}}
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
- name: POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: NODE_NAME
  valueFrom:
    fieldRef:
      fieldPath: spec.nodeName
{{- if .Values.observability.otel.enabled }}
- name: OTEL_SERVICE_NAME
  value: {{ .Values.observability.otel.serviceName | quote }}
- name: OTEL_SERVICE_VERSION
  value: {{ .Values.observability.otel.serviceVersion | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: "service.name={{ .Values.observability.otel.serviceName }},service.version={{ .Values.observability.otel.serviceVersion }},service.namespace=roost.birb.party,k8s.namespace.name=$(POD_NAMESPACE),k8s.pod.name=$(POD_NAME),k8s.node.name=$(NODE_NAME)"
{{- if .Values.observability.otel.exportPath }}
- name: OTEL_EXPORTER_FILE_PATH
  value: {{ .Values.observability.otel.exportPath | quote }}
{{- end }}
{{- if .Values.observability.otel.endpoint }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .Values.observability.otel.endpoint | quote }}
{{- end }}
{{- range $key, $value := .Values.observability.otel.headers }}
- name: OTEL_EXPORTER_OTLP_HEADERS_{{ $key | upper }}
  value: {{ $value | quote }}
{{- end }}
{{- end }}
{{- if .Values.highAvailability.enabled }}
- name: HA_ENABLED
  value: "true"
- name: HA_REPLICA_COUNT
  value: {{ .Values.controller.replicas | quote }}
{{- end }}
{{- if .Values.performance.enabled }}
- name: WORKER_POOL_SIZE
  value: {{ .Values.performance.workerPool.size | quote }}
- name: CACHE_SIZE
  value: {{ .Values.performance.cache.size | quote }}
- name: CACHE_TTL
  value: {{ .Values.performance.cache.ttl | quote }}
{{- end }}
{{- with .Values.controller.env }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}

{{/*
Create volume mounts
*/}}
{{- define "roost-keeper.volumeMounts" -}}
{{- if .Values.webhook.enabled }}
- name: webhook-certs
  mountPath: /tmp/k8s-webhook-server/serving-certs
  readOnly: true
{{- end }}
- name: tmp
  mountPath: /tmp
{{- if .Values.observability.enabled }}
- name: observability-data
  mountPath: {{ .Values.observability.otel.exportPath | default "/tmp/telemetry" }}
{{- end }}
{{- with .Values.controller.extraVolumeMounts }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}

{{/*
Create volumes
*/}}
{{- define "roost-keeper.volumes" -}}
{{- if .Values.webhook.enabled }}
- name: webhook-certs
  secret:
    defaultMode: 420
    secretName: {{ include "roost-keeper.fullname" . }}-webhook-certs
    optional: true
{{- end }}
- name: tmp
  emptyDir:
    sizeLimit: 100Mi
{{- if .Values.observability.enabled }}
- name: observability-data
  emptyDir:
    sizeLimit: 1Gi
{{- end }}
{{- with .Values.controller.extraVolumes }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}

{{/*
Create webhook certificate name
*/}}
{{- define "roost-keeper.webhookCertificateName" -}}
{{ include "roost-keeper.fullname" . }}-webhook-cert
{{- end }}

{{/*
Create webhook secret name
*/}}
{{- define "roost-keeper.webhookSecretName" -}}
{{ include "roost-keeper.fullname" . }}-webhook-certs
{{- end }}

{{/*
Create metrics service name
*/}}
{{- define "roost-keeper.metricsServiceName" -}}
{{ include "roost-keeper.fullname" . }}-metrics
{{- end }}

{{/*
Create webhook service name
*/}}
{{- define "roost-keeper.webhookServiceName" -}}
{{ include "roost-keeper.fullname" . }}-webhook
{{- end }}

{{/*
Create service monitor name
*/}}
{{- define "roost-keeper.serviceMonitorName" -}}
{{ include "roost-keeper.fullname" . }}
{{- end }}

{{/*
Create pod disruption budget name
*/}}
{{- define "roost-keeper.podDisruptionBudgetName" -}}
{{ include "roost-keeper.fullname" . }}
{{- end }}

{{/*
Create network policy name
*/}}
{{- define "roost-keeper.networkPolicyName" -}}
{{ include "roost-keeper.fullname" . }}
{{- end }}

{{/*
Validate configuration
*/}}
{{- define "roost-keeper.validateConfig" -}}
{{- if and .Values.webhook.enabled .Values.webhook.certManager.enabled (not .Values.certManager.enabled) }}
{{- fail "webhook.certManager.enabled requires certManager.enabled to be true" }}
{{- end }}
{{- if and .Values.monitoring.serviceMonitor.enabled (not .Values.monitoring.enabled) }}
{{- fail "monitoring.serviceMonitor.enabled requires monitoring.enabled to be true" }}
{{- end }}
{{- if and .Values.highAvailability.enabled (lt (.Values.controller.replicas | int) 2) }}
{{- fail "highAvailability.enabled requires controller.replicas to be at least 2" }}
{{- end }}
{{- if and .Values.controller.leaderElection.enabled (eq (.Values.controller.replicas | int) 1) }}
{{- printf "Warning: leader election is enabled but only 1 replica is configured" | fail }}
{{- end }}
{{- end }}


{{/*
Create RBAC proxy configuration
*/}}
{{- define "roost-keeper.rbacProxyArgs" -}}
- "--secure-listen-address=0.0.0.0:8443"
- "--upstream=http://127.0.0.1:8080/"
- "--logtostderr=true"
- "--v=0"
{{- end }}

{{/*
Network policy ingress rules
*/}}
{{- define "roost-keeper.networkPolicyIngress" -}}
{{- if .Values.security.networkPolicies.enabled }}
- from:
  - namespaceSelector:
      matchLabels:
        name: {{ include "roost-keeper.namespace" . }}
  ports:
  - protocol: TCP
    port: 8080
  - protocol: TCP
    port: 8081
{{- if .Values.webhook.enabled }}
  - protocol: TCP
    port: {{ .Values.webhook.port }}
{{- end }}
{{- if .Values.security.networkPolicies.allowMonitoring }}
- from:
  - namespaceSelector:
      matchLabels:
        name: {{ .Values.monitoring.prometheus.namespace }}
  ports:
  - protocol: TCP
    port: 8080
{{- end }}
{{- with .Values.security.networkPolicies.additionalIngress }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Network policy egress rules
*/}}
{{- define "roost-keeper.networkPolicyEgress" -}}
{{- if .Values.security.networkPolicies.enabled }}
- to: []
  ports:
  - protocol: TCP
    port: 53
  - protocol: UDP
    port: 53
- to: []
  ports:
  - protocol: TCP
    port: 443
  - protocol: TCP
    port: 6443
{{- with .Values.security.networkPolicies.additionalEgress }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}
{{- end }}
