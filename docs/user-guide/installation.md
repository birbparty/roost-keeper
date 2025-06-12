# Installation Guide

This guide covers all methods for installing Roost-Keeper in various environments, from development setups to production-ready deployments.

## Prerequisites

Before installing Roost-Keeper, ensure you have:

- **Kubernetes cluster** (v1.20+)
- **Helm** 3.8+ installed and configured
- **kubectl** configured for your cluster
- **Cluster-admin permissions** (for initial installation)
- **Container registry access** (for custom deployments)

### Verify Prerequisites

```bash
# Check Kubernetes version
kubectl version --short

# Check Helm version
helm version --short

# Verify cluster access
kubectl cluster-info

# Check permissions
kubectl auth can-i "*" "*" --all-namespaces
```

## Quick Installation (Recommended)

### 1. Add Helm Repository

```bash
helm repo add roost-keeper https://charts.roost-keeper.io
helm repo update
```

### 2. Install with Default Configuration

```bash
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace
```

### 3. Verify Installation

```bash
# Check pod status
kubectl get pods -n roost-keeper-system

# Expected output:
# NAME                                        READY   STATUS    RESTARTS   AGE
# roost-keeper-controller-7d4b8c8b4d-abc123   1/1     Running   0          2m
# roost-keeper-controller-7d4b8c8b4d-def456   1/1     Running   0          2m
# roost-keeper-controller-7d4b8c8b4d-ghi789   1/1     Running   0          2m

# Check CRDs
kubectl get crd | grep roost-keeper

# Test with a simple roost
kubectl apply -f https://raw.githubusercontent.com/birbparty/roost-keeper/main/examples/basic/nginx.yaml
```

## Environment-Specific Installations

### Development Environment

For local development and testing:

```bash
# Install with development values
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace \
  --values https://raw.githubusercontent.com/birbparty/roost-keeper/main/helm/roost-keeper/values-development.yaml
```

**Development configuration includes:**
- Single controller replica
- Debug logging enabled
- Reduced resource requirements
- Simplified security policies
- Local development tools integration

### Staging Environment

For staging and pre-production testing:

```bash
# Create custom values file
cat <<EOF > staging-values.yaml
controller:
  replicas: 2
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

monitoring:
  enabled: true
  prometheus:
    enabled: true

security:
  podSecurityStandards:
    enforce: baseline
  networkPolicies:
    enabled: true

observability:
  logging:
    level: info
  tracing:
    enabled: true
    samplingRate: "0.1"
EOF

# Install with staging configuration
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace \
  --values staging-values.yaml
```

### Production Environment

For production deployments with high availability:

```bash
# Create production values file
cat <<EOF > production-values.yaml
controller:
  replicas: 3
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: 1000m
      memory: 1Gi
  
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - roost-keeper
        topologyKey: kubernetes.io/hostname

monitoring:
  enabled: true
  prometheus:
    enabled: true
  grafana:
    enabled: true

security:
  podSecurityStandards:
    enforce: restricted
  networkPolicies:
    enabled: true
  
highAvailability:
  enabled: true
  podDisruptionBudget:
    enabled: true
    minAvailable: 2

observability:
  enabled: true
  logging:
    level: info
    structured: true
  tracing:
    enabled: true
    samplingRate: "0.05"
  metrics:
    enabled: true
    interval: 15s

backup:
  enabled: true
  schedule: "0 2 * * *"
  retention: "30d"
EOF

# Install with production configuration
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace \
  --values production-values.yaml
```

## Advanced Installation Options

### Custom Container Registry

If using a private container registry:

```bash
# Create image pull secret
kubectl create secret docker-registry roost-keeper-registry \
  --docker-server=your-registry.company.com \
  --docker-username=your-username \
  --docker-password=your-password \
  --docker-email=your-email@company.com \
  --namespace roost-keeper-system

# Install with custom registry
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace \
  --set global.imageRegistry=your-registry.company.com \
  --set global.imagePullSecrets[0].name=roost-keeper-registry
```

### Air-Gapped Environment

For environments without internet access:

```bash
# 1. Download and push images to local registry
docker pull roost-keeper/controller:latest
docker tag roost-keeper/controller:latest your-local-registry/roost-keeper/controller:latest
docker push your-local-registry/roost-keeper/controller:latest

# 2. Download Helm chart
helm fetch roost-keeper/roost-keeper --untar

# 3. Modify values.yaml to use local registry
sed -i 's|roost-keeper/|your-local-registry/roost-keeper/|g' roost-keeper/values.yaml

# 4. Install from local chart
helm install roost-keeper ./roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace
```

### Multi-Cluster Setup

For managing multiple clusters:

```bash
# Install on each cluster with cluster-specific configuration
cat <<EOF > cluster-east-values.yaml
global:
  clusterName: "east"
  region: "us-east-1"

controller:
  extraEnvVars:
    - name: CLUSTER_REGION
      value: "us-east-1"
    - name: CLUSTER_NAME
      value: "east"

observability:
  tracing:
    endpoint:
      url: "https://tracing.company.com"
  metrics:
    remoteWrite:
      url: "https://prometheus.company.com/api/v1/write"
EOF

helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace \
  --values cluster-east-values.yaml
```

## Configuration Options

### Core Configuration

```yaml
# values.yaml
controller:
  # Number of controller replicas
  replicas: 3
  
  # Resource limits and requests
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: 1000m
      memory: 1Gi
  
  # Node selection
  nodeSelector:
    kubernetes.io/os: linux
  
  # Tolerations for dedicated nodes
  tolerations:
    - key: "roost-keeper"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"

# Webhook configuration
webhook:
  enabled: true
  port: 9443
  
  # Certificate management
  certManager:
    enabled: true
  
  # Manual certificate configuration
  certificates:
    secret: webhook-certs
    caBundle: ""

# Service configuration
service:
  type: ClusterIP
  port: 443
  
  # For LoadBalancer type
  # type: LoadBalancer
  # loadBalancerIP: "10.0.0.100"
```

### Security Configuration

```yaml
security:
  # Pod Security Standards
  podSecurityStandards:
    enforce: restricted
    audit: restricted
    warn: restricted
  
  # Network Policies
  networkPolicies:
    enabled: true
    ingress:
      - from:
        - namespaceSelector:
            matchLabels:
              name: monitoring
        ports:
        - protocol: TCP
          port: 8080
  
  # RBAC configuration
  rbac:
    create: true
    
  # Service Account
  serviceAccount:
    create: true
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/roost-keeper
```

### Monitoring Configuration

```yaml
monitoring:
  enabled: true
  
  # Prometheus integration
  prometheus:
    enabled: true
    serviceMonitor:
      enabled: true
      labels:
        team: platform
        component: roost-keeper
  
  # Grafana dashboards
  grafana:
    enabled: true
    dashboards:
      enabled: true
      
  # Custom metrics
  metrics:
    enabled: true
    port: 8080
    interval: 15s
```

## Verification Steps

### 1. Controller Health Check

```bash
# Check controller logs
kubectl logs -n roost-keeper-system deployment/roost-keeper-controller

# Check controller metrics
kubectl port-forward -n roost-keeper-system svc/roost-keeper-controller-service 8080:8080
curl http://localhost:8080/metrics | grep roost_keeper
```

### 2. Webhook Verification

```bash
# Check webhook configuration
kubectl get validatingwebhookconfiguration roost-keeper-validating-webhook -o yaml

# Test webhook with dry-run
kubectl apply --dry-run=server -f - <<EOF
apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: test-webhook
  namespace: default
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
    version: "13.2.23"
EOF
```

### 3. CRD Validation

```bash
# List CRDs
kubectl get crd | grep roost-keeper

# Describe CRD
kubectl describe crd managedroosts.roost-keeper.io

# Test CRD with explain
kubectl explain managedroost.spec.healthChecks
```

### 4. End-to-End Test

```bash
# Apply a test roost
kubectl apply -f - <<EOF
apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: test-installation
  namespace: default
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
    version: "13.2.23"
  healthChecks:
    - name: http-check
      type: http
      http:
        url: "http://test-installation.default.svc.cluster.local"
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 5m
EOF

# Watch deployment progress
kubectl get managedroost test-installation -w

# Check roost status
kubectl describe managedroost test-installation

# Verify Helm release
helm list -A

# Clean up
kubectl delete managedroost test-installation
```

## Troubleshooting Installation

### Common Issues

#### 1. Insufficient Permissions

**Error:** `unable to create CRDs`

**Solution:**
```bash
# Verify cluster-admin access
kubectl auth can-i "*" "*" --all-namespaces

# If using RBAC, ensure proper permissions
kubectl create clusterrolebinding cluster-admin-binding \
  --clusterrole=cluster-admin \
  --user=$(kubectl config current-context)
```

#### 2. Webhook Certificate Issues

**Error:** `x509: certificate signed by unknown authority`

**Solution:**
```bash
# Check cert-manager installation
kubectl get pods -n cert-manager

# If cert-manager not available, disable webhook temporarily
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --set webhook.enabled=false

# Or provide manual certificates
kubectl create secret tls webhook-certs \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  --namespace roost-keeper-system
```

#### 3. Image Pull Issues

**Error:** `ImagePullBackOff`

**Solution:**
```bash
# Check image pull secrets
kubectl get secrets -n roost-keeper-system

# Create image pull secret if needed
kubectl create secret docker-registry roost-keeper-registry \
  --docker-server=registry.company.com \
  --docker-username=username \
  --docker-password=password \
  --namespace roost-keeper-system

# Update deployment to use secret
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --set global.imagePullSecrets[0].name=roost-keeper-registry
```

#### 4. Resource Constraints

**Error:** `Insufficient cpu/memory`

**Solution:**
```bash
# Check node resources
kubectl describe nodes

# Reduce resource requirements
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --set controller.resources.requests.cpu=50m \
  --set controller.resources.requests.memory=64Mi
```

#### 5. Namespace Issues

**Error:** `namespace not found`

**Solution:**
```bash
# Create namespace manually
kubectl create namespace roost-keeper-system

# Or ensure Helm creates it
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace
```

## Upgrade Guide

### Standard Upgrade

```bash
# Update repository
helm repo update

# Check available versions
helm search repo roost-keeper/roost-keeper --versions

# Upgrade to latest version
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system

# Verify upgrade
kubectl get pods -n roost-keeper-system
helm history roost-keeper -n roost-keeper-system
```

### Upgrade with Configuration Changes

```bash
# Backup current configuration
helm get values roost-keeper -n roost-keeper-system > current-values.yaml

# Apply new configuration during upgrade
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --values updated-values.yaml

# Rollback if needed
helm rollback roost-keeper 1 -n roost-keeper-system
```

## Uninstallation

### Complete Removal

```bash
# Remove Helm release (preserves CRDs by default)
helm uninstall roost-keeper -n roost-keeper-system

# Remove ManagedRoosts (if any remain)
kubectl get managedroosts -A
kubectl delete managedroosts --all -A

# Remove CRDs
kubectl delete crd managedroosts.roost-keeper.io

# Remove webhook configurations
kubectl delete validatingwebhookconfiguration roost-keeper-validating-webhook
kubectl delete mutatingwebhookconfiguration roost-keeper-mutating-webhook

# Remove namespace
kubectl delete namespace roost-keeper-system
```

### Safe Removal (Preserve Data)

```bash
# Scale down controller (stops new operations)
kubectl scale deployment roost-keeper-controller --replicas=0 -n roost-keeper-system

# Wait for ongoing operations to complete
kubectl get managedroosts -A --watch

# Remove controller but keep CRDs and resources
helm uninstall roost-keeper -n roost-keeper-system --keep-history

# CRDs and ManagedRoosts remain for manual cleanup
```

## Next Steps

After successful installation:

1. 📖 [Read the Configuration Guide](configuration.md)
2. 🎯 [Follow the Getting Started Tutorial](../tutorials/getting-started.md)
3. 🔍 [Explore Example Configurations](../examples/)
4. 📊 [Set up Monitoring](monitoring.md)
5. 🔒 [Configure Security](security.md)

## Getting Help

If you encounter issues during installation:

- 📚 [Check the Troubleshooting Guide](../troubleshooting/common-issues.md)
- 💬 [Join Community Discussions](https://github.com/birbparty/roost-keeper/discussions)
- 🐛 [Report Installation Issues](https://github.com/birbparty/roost-keeper/issues)
- 📧 [Contact Support](mailto:support@roost-keeper.io)
