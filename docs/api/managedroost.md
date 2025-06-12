# ManagedRoost API Reference

The `ManagedRoost` is the core Custom Resource Definition (CRD) for Roost-Keeper, enabling declarative management of Helm chart lifecycles with intelligent health monitoring and automated teardown policies.

## API Version

- **API Version**: `roost.birb.party/v1alpha1`
- **Kind**: `ManagedRoost`
- **Scope**: `Namespaced`

## Resource Structure

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: string
  namespace: string
  labels: map[string]string
  annotations: map[string]string
spec:
  # Core configuration
  chart: ChartSpec
  healthChecks: []HealthCheckSpec
  teardownPolicy: TeardownPolicySpec
  
  # Optional features
  tenancy: TenancySpec
  observability: ObservabilitySpec
  metadata: map[string]string
  namespace: string
status:
  phase: string
  health: string
  # ... detailed status fields
```

## Specification Reference

### ManagedRoostSpec

The main specification for a ManagedRoost resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `chart` | [ChartSpec](#chartspec) | Yes | Helm chart configuration |
| `healthChecks` | [][HealthCheckSpec](#healthcheckspec) | No | Health monitoring configuration |
| `teardownPolicy` | [TeardownPolicySpec](#teardownpolicyspec) | No | Automated teardown policies |
| `tenancy` | [TenancySpec](#tenancyspec) | No | Multi-tenant configuration |
| `observability` | [ObservabilitySpec](#observabilityspec) | No | Observability settings |
| `metadata` | `map[string]string` | No | Operational metadata |
| `namespace` | `string` | No | Target namespace for deployment |

### ChartSpec

Defines the Helm chart to deploy and manage.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repository` | [ChartRepositorySpec](#chartrepositoryspec) | Yes | Repository configuration |
| `name` | `string` | Yes | Chart name (must match `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`) |
| `version` | `string` | Yes | Chart version or version constraint |
| `values` | [ChartValuesSpec](#chartvaluesspec) | No | Values override configuration |
| `upgradePolicy` | [UpgradePolicySpec](#upgradepolicyspec) | No | Helm upgrade behavior |
| `hooks` | [HookPolicySpec](#hookpolicyspec) | No | Helm hooks configuration |

#### Example: Basic Chart Configuration

```yaml
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
      type: http
    version: "13.2.23"
    values:
      inline: |
        replicaCount: 2
        service:
          type: ClusterIP
```

#### Example: OCI Registry

```yaml
spec:
  chart:
    name: redis
    repository:
      url: oci://registry-1.docker.io/bitnamicharts
      type: oci
    version: "18.19.4"
```

### ChartRepositorySpec

Configures access to Helm chart repositories.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | `string` | Yes | - | Repository URL (must match `^(https?|oci|git\+https)://.*$`) |
| `type` | `string` | No | `http` | Repository type (`http`, `oci`, `git`) |
| `auth` | [RepositoryAuthSpec](#repositoryauthspec) | No | - | Authentication for private repositories |
| `tls` | [RepositoryTLSSpec](#repositorytlsspec) | No | - | TLS configuration |

#### Example: Private Repository with Authentication

```yaml
chart:
  repository:
    url: https://private-charts.company.com
    type: http
    auth:
      secretRef:
        name: chart-repo-credentials
        namespace: default
    tls:
      insecureSkipVerify: false
```

### RepositoryAuthSpec

Authentication configuration for private repositories.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secretRef` | [SecretReference](#secretreference) | No | Secret containing credentials |
| `username` | `string` | No | Username for basic auth |
| `password` | `string` | No | Password for basic auth |
| `token` | `string` | No | Token for token-based auth (OCI registries) |
| `type` | `string` | No | Authentication type (`basic`, `token`, `docker-config`) |

### ChartValuesSpec

Defines how to override chart values.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `inline` | `string` | No | Inline YAML values |
| `configMapRefs` | [][ValueSourceRef](#valuesourceref) | No | Values from ConfigMaps |
| `secretRefs` | [][ValueSourceRef](#valuesourceref) | No | Values from Secrets |
| `template` | [ValueTemplateSpec](#valuetemplatespec) | No | Template processing configuration |

#### Example: Multiple Value Sources

```yaml
chart:
  values:
    inline: |
      replicaCount: 2
      service:
        type: ClusterIP
    configMapRefs:
      - name: app-config
        key: values.yaml
    secretRefs:
      - name: db-credentials
        key: database.yaml
    template:
      enabled: true
      context:
        environment: "production"
        region: "us-west-2"
```

### HealthCheckSpec

Defines health monitoring configuration for the deployed application.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | `string` | Yes | - | Health check identifier |
| `type` | `string` | Yes | - | Check type (`http`, `tcp`, `udp`, `grpc`, `prometheus`, `kubernetes`) |
| `interval` | `duration` | No | `30s` | Check interval |
| `timeout` | `duration` | No | `10s` | Individual check timeout |
| `failureThreshold` | `int32` | No | `3` | Consecutive failures before unhealthy |
| `weight` | `int32` | No | `1` | Weight for composite evaluation (0-100) |

#### Health Check Types

##### HTTP Health Checks

```yaml
healthChecks:
  - name: api-health
    type: http
    http:
      url: "http://my-app.default.svc.cluster.local/health"
      method: GET  # GET, POST, PUT, HEAD
      expectedCodes: [200, 202]
      expectedBody: "OK"
      headers:
        Authorization: "Bearer token"
        User-Agent: "roost-keeper"
      tls:
        insecureSkipVerify: false
    interval: 30s
    timeout: 10s
    failureThreshold: 3
    weight: 2
```

##### TCP Health Checks

```yaml
healthChecks:
  - name: database-tcp
    type: tcp
    tcp:
      host: postgres.default.svc.cluster.local
      port: 5432
      sendData: "ping"  # Optional protocol validation
      expectedResponse: "pong"
      connectionTimeout: 5s
      enablePooling: true
    interval: 20s
    timeout: 5s
```

##### UDP Health Checks

```yaml
healthChecks:
  - name: dns-udp
    type: udp
    udp:
      host: dns-service.kube-system.svc.cluster.local
      port: 53
      sendData: "health-check"
      expectedResponse: "ok"
      readTimeout: 3s
      retries: 3
    interval: 30s
```

##### gRPC Health Checks

```yaml
healthChecks:
  - name: grpc-service
    type: grpc
    grpc:
      host: grpc-service.default.svc.cluster.local
      port: 9090
      service: "my.service.Health"  # Optional service name
      tls:
        enabled: true
        insecureSkipVerify: false
        serverName: grpc-service.company.com
      auth:
        jwt: "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9..."
      connectionPool:
        enabled: true
        maxConnections: 10
        maxIdleTime: 60s
      enableStreaming: true
    interval: 30s
```

##### Prometheus Health Checks

```yaml
healthChecks:
  - name: error-rate
    type: prometheus
    prometheus:
      query: 'rate(http_requests_total{status=~"5.."}[5m])'
      threshold: "0.01"
      operator: lt  # lt, lte, gt, gte, eq, ne
      endpoint: "http://prometheus.monitoring.svc.cluster.local:9090"
      serviceDiscovery:
        serviceName: prometheus
        serviceNamespace: monitoring
        servicePort: "9090"
      auth:
        bearerToken: "token"
      queryTimeout: 30s
      cacheTTL: 5m
      trendAnalysis:
        enabled: true
        timeWindow: 15m
        improvementThreshold: 10
        degradationThreshold: 20
    interval: 60s
```

##### Kubernetes Native Health Checks

```yaml
healthChecks:
  - name: k8s-resources
    type: kubernetes
    kubernetes:
      resources:
        - apiVersion: v1
          kind: Pod
          labelSelector:
            app: my-app
          conditions:
            - type: Ready
              status: "True"
        - apiVersion: apps/v1
          kind: Deployment
          labelSelector:
            app: my-app
          conditions:
            - type: Available
              status: "True"
        - apiVersion: v1
          kind: Service
          labelSelector:
            app: my-app
      requiredReadyRatio: 0.8  # 80% of pods must be ready
      apiTimeout: 5s
      enableCaching: true
      cacheTTL: 30s
      checkPodRestarts: true
      maxPodRestarts: 5
      checkResourceLimits: true
      checkNodeHealth: false
    interval: 15s
```

### TeardownPolicySpec

Defines automated teardown policies for the managed application.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `triggers` | [][TeardownTriggerSpec](#teardowntriggerspec) | No | - | Automatic teardown triggers |
| `requireManualApproval` | `bool` | No | `false` | Require manual approval for teardown |
| `dataPreservation` | [DataPreservationSpec](#datapreservationspec) | No | - | Data preservation policies |
| `cleanup` | [CleanupSpec](#cleanupspec) | No | - | Cleanup configuration |

#### Teardown Triggers

##### Timeout-Based Teardown

```yaml
teardownPolicy:
  triggers:
    - type: timeout
      timeout: 2h  # Tear down after 2 hours
```

##### Health-Based Teardown

```yaml
teardownPolicy:
  triggers:
    - type: failure_count
      failureCount: 10  # Tear down after 10 consecutive health check failures
```

##### Resource-Based Teardown

```yaml
teardownPolicy:
  triggers:
    - type: resource_threshold
      resourceThreshold:
        cpu: "10m"      # Low CPU usage
        memory: "50Mi"  # Low memory usage
        storage: "1Gi"  # Low storage usage
        custom:
          network_io: "1kbps"
```

##### Schedule-Based Teardown

```yaml
teardownPolicy:
  triggers:
    - type: schedule
      schedule: "0 18 * * 5"  # Every Friday at 6 PM (cron format)
```

##### Webhook-Based Teardown

```yaml
teardownPolicy:
  triggers:
    - type: webhook
      webhook:
        url: "https://api.company.com/teardown-approval"
        method: POST
        headers:
          Authorization: "Bearer token"
        auth:
          bearerToken: "webhook-token"
```

#### Data Preservation

```yaml
teardownPolicy:
  dataPreservation:
    enabled: true
    preserveResources:
      - "PersistentVolumeClaim"
      - "Secret"
      - "ConfigMap"
    backupPolicy:
      schedule: "0 2 * * *"  # Daily at 2 AM
      retention: "7d"
      storage:
        type: s3
        config:
          bucket: "backups"
          region: "us-west-2"
```

### TenancySpec

Multi-tenant configuration for namespace isolation and resource management.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tenantId` | `string` | No | Tenant identifier |
| `namespaceIsolation` | [NamespaceIsolationSpec](#namespaceisolationspec) | No | Namespace isolation config |
| `rbac` | [RBACSpec](#rbacspec) | No | RBAC configuration |
| `networkPolicy` | [NetworkPolicySpec](#networkpolicyspec) | No | Network policies |
| `resourceQuota` | [ResourceQuotaSpec](#resourcequotaspec) | No | Resource quotas |

#### Example: Complete Tenancy Configuration

```yaml
spec:
  tenancy:
    tenantId: "team-alpha"
    namespaceIsolation:
      enabled: true
      prefix: "alpha-"
      labels:
        tenant: "team-alpha"
        cost-center: "engineering"
      annotations:
        roost-keeper.io/tenant: "team-alpha"
    rbac:
      enabled: true
      tenantId: "team-alpha"
      identityProvider: "oidc"
      serviceAccounts:
        - name: "alpha-deployer"
          annotations:
            eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/alpha-deployer"
      roles:
        - name: "alpha-operator"
          type: "Role"
          template: "namespace-operator"
      roleBindings:
        - name: "alpha-team-binding"
          subjects:
            - kind: "Group"
              name: "alpha-developers"
          roleRef:
            kind: "Role"
            name: "alpha-operator"
    networkPolicy:
      enabled: true
      ingress:
        - from:
          - namespaceSelector:
              matchLabels:
                name: monitoring
          ports:
          - protocol: TCP
            port: 8080
    resourceQuota:
      enabled: true
      hard:
        requests.cpu: "4"
        requests.memory: "8Gi"
        limits.cpu: "8"
        limits.memory: "16Gi"
        persistentvolumeclaims: "10"
```

### ObservabilitySpec

Configuration for metrics, logging, and tracing.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `metrics` | [MetricsSpec](#metricsspec) | No | Metrics collection config |
| `tracing` | [TracingSpec](#tracingspec) | No | Distributed tracing config |
| `logging` | [LoggingSpec](#loggingspec) | No | Logging configuration |

#### Example: Comprehensive Observability

```yaml
spec:
  observability:
    metrics:
      enabled: true
      interval: 15s
      custom:
        - name: "app_requests_per_second"
          query: 'rate(http_requests_total[1m])'
          type: gauge
        - name: "app_error_rate"
          query: 'rate(http_requests_total{status=~"5.."}[5m])'
          type: gauge
    tracing:
      enabled: true
      samplingRate: "0.1"
      endpoint:
        url: "https://jaeger.company.com/api/traces"
        auth:
          bearerToken: "tracing-token"
    logging:
      level: info
      structured: true
      outputs:
        - type: http
          config:
            url: "https://logs.company.com/api/v1/logs"
            headers: '{"Authorization": "Bearer log-token"}'
        - type: file
          config:
            path: "/var/log/roost-keeper"
            maxSize: "100MB"
```

## Status Reference

### ManagedRoostStatus

The observed state of a ManagedRoost resource.

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | Current phase (`Pending`, `Deploying`, `Ready`, `Failed`, `TearingDown`) |
| `health` | `string` | Overall health (`healthy`, `unhealthy`, `degraded`, `unknown`) |
| `helmRelease` | [HelmReleaseStatus](#helmreleasestatus) | Helm release information |
| `healthChecks` | [][HealthCheckStatus](#healthcheckstatus) | Individual health check results |
| `teardown` | [TeardownStatus](#teardownstatus) | Teardown status |
| `conditions` | [][Condition](#condition) | Standard Kubernetes conditions |
| `observability` | [ObservabilityStatus](#observabilitystatus) | Observability status |
| `lifecycle` | [LifecycleStatus](#lifecyclestatus) | Lifecycle tracking |
| `lastUpdateTime` | `metav1.Time` | Last status update |
| `observedGeneration` | `int64` | Generation observed by controller |
| `lastReconcileTime` | `metav1.Time` | Last reconciliation time |

### Status Examples

#### Healthy ManagedRoost

```yaml
status:
  phase: Ready
  health: healthy
  helmRelease:
    name: my-app
    revision: 1
    status: deployed
    lastDeployed: "2024-01-15T10:30:00Z"
  healthChecks:
    - name: http-health
      status: healthy
      lastCheck: "2024-01-15T10:35:00Z"
      failureCount: 0
    - name: kubernetes-pods
      status: healthy
      lastCheck: "2024-01-15T10:35:00Z"
      failureCount: 0
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: HealthChecksPassi​ng
      message: All health checks are passing
  observedGeneration: 1
  lastReconcileTime: "2024-01-15T10:35:00Z"
```

#### Unhealthy ManagedRoost

```yaml
status:
  phase: Ready
  health: unhealthy
  healthChecks:
    - name: http-health
      status: unhealthy
      lastCheck: "2024-01-15T10:35:00Z"
      failureCount: 5
      message: "connection refused"
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:33:00Z"
      reason: HealthChecksFailing
      message: HTTP health check failing for 5 consecutive attempts
```

## Usage Patterns

### Development Environment

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: feature-branch-app
  namespace: development
  labels:
    environment: dev
    branch: feature/new-api
spec:
  chart:
    name: myapp
    repository:
      url: https://charts.company.com
    version: "1.0.0"
  healthChecks:
    - name: readiness
      type: http
      http:
        url: "http://feature-branch-app.development.svc.cluster.local/ready"
      interval: 30s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 4h  # Auto-cleanup after 4 hours
      - type: failure_count
        failureCount: 3
```

### Staging Environment

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: staging-deployment
  namespace: staging
spec:
  chart:
    name: myapp
    repository:
      url: https://charts.company.com
    version: "1.2.0"
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://staging-deployment.staging.svc.cluster.local/health"
      interval: 30s
      weight: 2
    - name: database-connectivity
      type: tcp
      tcp:
        host: postgres.staging.svc.cluster.local
        port: 5432
      interval: 60s
      weight: 1
  teardownPolicy:
    triggers:
      - type: schedule
        schedule: "0 2 * * *"  # Daily cleanup at 2 AM
    requireManualApproval: true
  observability:
    metrics:
      enabled: true
    tracing:
      enabled: true
      samplingRate: "0.5"
```

### Production Canary

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: canary-deployment
  namespace: production
spec:
  chart:
    name: myapp
    repository:
      url: https://charts.company.com
    version: "2.0.0"
    values:
      inline: |
        replicaCount: 1
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
        canary:
          enabled: true
          weight: 5  # 5% traffic
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://canary-deployment.production.svc.cluster.local/health"
      interval: 15s
      failureThreshold: 2
      weight: 3
    - name: error-rate
      type: prometheus
      prometheus:
        query: 'rate(http_requests_total{job="canary-deployment",status=~"5.."}[5m])'
        threshold: "0.01"
        operator: lt
      interval: 30s
      weight: 2
  teardownPolicy:
    triggers:
      - type: failure_count
        failureCount: 5
      - type: timeout
        timeout: 24h
    requireManualApproval: true
    dataPreservation:
      enabled: true
```

## Validation and Constraints

### Field Validation

- **Chart Name**: Must match regex `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
- **Repository URL**: Must match regex `^(https?|oci|git\+https)://.*$`
- **Weights**: Must be between 0-100
- **Ports**: Must be between 1-65535
- **Failure Threshold**: Must be >= 1

### Resource Limits

- **Maximum Health Checks**: 20 per ManagedRoost
- **Maximum Teardown Triggers**: 10 per ManagedRoost
- **Maximum ConfigMap/Secret References**: 10 per values section

## Best Practices

### Health Check Design

1. **Use Multiple Check Types**: Combine HTTP, TCP, and Kubernetes checks for comprehensive monitoring
2. **Set Appropriate Weights**: Critical checks should have higher weights
3. **Configure Realistic Timeouts**: Balance responsiveness with false positives
4. **Use Prometheus for Business Logic**: Monitor application-specific metrics

### Teardown Policy Design

1. **Layer Multiple Triggers**: Combine timeout, health, and resource triggers
2. **Enable Data Preservation**: For stateful applications
3. **Use Manual Approval**: For production environments
4. **Configure Gradual Cleanup**: Appropriate grace periods

### Performance Optimization

1. **Cache Health Check Results**: Use appropriate intervals
2. **Optimize Prometheus Queries**: Use efficient PromQL
3. **Limit Concurrent Checks**: Avoid overwhelming target services
4. **Use Connection Pooling**: For gRPC and TCP checks

## Migration Guide

### From v1alpha1 to Future Versions

When migrating to newer API versions:

1. **Backup Current Resources**: Always backup before migration
2. **Test in Non-Production**: Validate migration in development first
3. **Use Gradual Rollout**: Migrate incrementally
4. **Monitor Health Checks**: Ensure compatibility during migration

This comprehensive API reference provides all the information needed to effectively use ManagedRoost resources in your Kubernetes cluster with Roost-Keeper.
