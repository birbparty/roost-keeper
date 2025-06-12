# Getting Started with Roost-Keeper

This tutorial will guide you through your first 30 minutes with Roost-Keeper, from installation to deploying and managing your first application with automated health monitoring and teardown policies.

## What You'll Learn

- How to install Roost-Keeper in your cluster
- How to create your first ManagedRoost with health checks
- How to monitor application health and lifecycle
- How to configure automatic teardown policies
- How to troubleshoot common issues
- How to use the CLI tools for better productivity

## Prerequisites

- **Kubernetes cluster** (v1.20+) with kubectl access
- **Helm 3.8+** installed
- **Cluster-admin permissions** for installation
- **Basic Kubernetes knowledge** (pods, services, deployments)

### Verify Prerequisites

```bash
# Check Kubernetes version
kubectl version --short

# Check Helm version
helm version --short

# Verify cluster access and permissions
kubectl auth can-i "*" "*" --all-namespaces
```

## Step 1: Install Roost-Keeper

### Quick Installation

```bash
# Add the Helm repository
helm repo add roost-keeper https://charts.roost-keeper.io
helm repo update

# Install Roost-Keeper
helm install roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --create-namespace

# Wait for installation to complete
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=roost-keeper \
  -n roost-keeper-system \
  --timeout=300s
```

### Verify Installation

```bash
# Check pod status
kubectl get pods -n roost-keeper-system

# Expected output:
# NAME                                        READY   STATUS    RESTARTS   AGE
# roost-keeper-controller-7d4b8c8b4d-abc123   1/1     Running   0          2m
# roost-keeper-controller-7d4b8c8b4d-def456   1/1     Running   0          2m
# roost-keeper-controller-7d4b8c8b4d-ghi789   1/1     Running   0          2m

# Check CRDs
kubectl get crd managedroosts.roost.birb.party

# Test CRD functionality
kubectl explain managedroost.spec
```

## Step 2: Create Your First ManagedRoost

Let's deploy a simple NGINX application using Roost-Keeper with health monitoring and automatic teardown.

### Create the ManagedRoost Resource

```yaml
# my-first-roost.yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: my-first-nginx
  namespace: default
  labels:
    app: nginx
    environment: tutorial
    managed-by: roost-keeper
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
      type: http
    version: "13.2.23"
    values:
      inline: |
        service:
          type: ClusterIP
          port: 80
        replicaCount: 2
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://my-first-nginx.default.svc.cluster.local"
        method: GET
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
      failureThreshold: 3
      weight: 1
    - name: kubernetes-pods
      type: kubernetes
      kubernetes:
        resources:
          - apiVersion: v1
            kind: Pod
            labelSelector:
              app.kubernetes.io/name: nginx
            conditions:
              - type: Ready
                status: "True"
      interval: 15s
      timeout: 5s
      failureThreshold: 2
      weight: 1
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 2h
    cleanup:
      gracePeriod: 30s
      force: true
  observability:
    metrics:
      enabled: true
      interval: 15s
    logging:
      level: info
      structured: true
```

### Apply the Configuration

```bash
# Create the ManagedRoost
kubectl apply -f my-first-roost.yaml

# Alternatively, apply directly from stdin
kubectl apply -f - <<EOF
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: my-first-nginx
  namespace: default
  labels:
    app: nginx
    environment: tutorial
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
      type: http
    version: "13.2.23"
    values:
      inline: |
        service:
          type: ClusterIP
          port: 80
        replicaCount: 2
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://my-first-nginx.default.svc.cluster.local"
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
      failureThreshold: 3
    - name: kubernetes-pods
      type: kubernetes
      kubernetes:
        resources:
          - apiVersion: v1
            kind: Pod
            labelSelector:
              app.kubernetes.io/name: nginx
            conditions:
              - type: Ready
                status: "True"
      interval: 15s
      timeout: 5s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 2h
EOF
```

## Step 3: Monitor Your Roost

### Watch the Deployment Progress

```bash
# Watch the roost status in real-time
kubectl get managedroost my-first-nginx -w

# You should see the phase progress:
# NAME             PHASE       HEALTH    CHART   VERSION    AGE
# my-first-nginx   Pending     unknown   nginx   13.2.23    5s
# my-first-nginx   Deploying   unknown   nginx   13.2.23    10s
# my-first-nginx   Ready       healthy   nginx   13.2.23    45s
```

### Check Detailed Status

```bash
# Get comprehensive status information
kubectl describe managedroost my-first-nginx

# Check health status specifically
kubectl get managedroost my-first-nginx -o jsonpath='{.status.health}' && echo

# View health check details
kubectl get managedroost my-first-nginx -o jsonpath='{.status.healthChecks}' | jq '.'
```

### View Generated Resources

```bash
# List all resources created by the roost
kubectl get all -l app.kubernetes.io/instance=my-first-nginx

# Check the Helm release
helm list -A | grep my-first-nginx

# Get Helm release details
helm status my-first-nginx -n default
```

## Step 4: Test Health Monitoring

### View Current Health Status

```bash
# Check overall health
kubectl get managedroost my-first-nginx -o jsonpath='{.status.health}' && echo

# View individual health check results
kubectl get managedroost my-first-nginx -o yaml | grep -A 20 healthChecks:
```

### Simulate Health Issues

Let's test how Roost-Keeper responds to health problems:

```bash
# Scale down pods to test Kubernetes health check
kubectl scale deployment my-first-nginx --replicas=0

# Watch how health status changes
watch "kubectl get managedroost my-first-nginx -o jsonpath='{.status.health}'"

# Check health check failure counts
kubectl get managedroost my-first-nginx -o jsonpath='{.status.healthChecks}' | jq '.'

# Scale back up to restore health
kubectl scale deployment my-first-nginx --replicas=2

# Wait for health to recover
kubectl wait --for=jsonpath='{.status.health}'=healthy managedroost my-first-nginx --timeout=120s
```

### Test HTTP Health Check

```bash
# Port-forward to test HTTP endpoint directly
kubectl port-forward svc/my-first-nginx 8080:80 &

# Test the endpoint
curl -I http://localhost:8080

# Stop port-forward
kill %1
```

## Step 5: Configure Advanced Features

### Add Multiple Health Check Types

Let's enhance our roost with more sophisticated health monitoring:

```bash
# Update the roost with additional health checks
kubectl apply -f - <<EOF
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: my-first-nginx
  namespace: default
  labels:
    app: nginx
    environment: tutorial
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
      type: http
    version: "13.2.23"
    values:
      inline: |
        service:
          type: ClusterIP
          port: 80
        replicaCount: 2
        metrics:
          enabled: true
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://my-first-nginx.default.svc.cluster.local"
        expectedCodes: [200]
        headers:
          User-Agent: "roost-keeper-health-check"
      interval: 30s
      timeout: 10s
      failureThreshold: 3
      weight: 2
    - name: tcp-connectivity
      type: tcp
      tcp:
        host: my-first-nginx.default.svc.cluster.local
        port: 80
      interval: 20s
      timeout: 5s
      failureThreshold: 2
      weight: 1
    - name: kubernetes-health
      type: kubernetes
      kubernetes:
        resources:
          - apiVersion: v1
            kind: Pod
            labelSelector:
              app.kubernetes.io/name: nginx
            conditions:
              - type: Ready
                status: "True"
          - apiVersion: apps/v1
            kind: Deployment
            labelSelector:
              app.kubernetes.io/name: nginx
            conditions:
              - type: Available
                status: "True"
      interval: 15s
      timeout: 5s
      failureThreshold: 2
      weight: 3
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 2h
      - type: failure_count
        failureCount: 10
    cleanup:
      gracePeriod: 30s
      force: true
  observability:
    metrics:
      enabled: true
      interval: 15s
    logging:
      level: info
      structured: true
    tracing:
      enabled: true
      samplingRate: "0.1"
EOF
```

### Configure Smart Teardown Policies

```bash
# Add resource-based teardown triggers
kubectl patch managedroost my-first-nginx --type='merge' -p='
{
  "spec": {
    "teardownPolicy": {
      "triggers": [
        {
          "type": "timeout",
          "timeout": "2h"
        },
        {
          "type": "failure_count",
          "failureCount": 5
        },
        {
          "type": "resource_threshold",
          "resourceThreshold": {
            "cpu": "10m",
            "memory": "50Mi"
          }
        }
      ],
      "cleanup": {
        "gracePeriod": "60s",
        "force": true,
        "order": ["Service", "Deployment", "Pod"]
      }
    }
  }
}'
```

## Step 6: Use the CLI Tools

### Install the CLI

```bash
# Download and install roost CLI (Linux)
curl -L https://github.com/birbparty/roost-keeper/releases/latest/download/roost-linux-amd64 -o roost
chmod +x roost
sudo mv roost /usr/local/bin/

# Or install as kubectl plugin
curl -L https://github.com/birbparty/roost-keeper/releases/latest/download/kubectl-roost-linux-amd64 -o kubectl-roost
chmod +x kubectl-roost
sudo mv kubectl-roost /usr/local/bin/

# Verify installation
roost version
```

### Use CLI Commands

```bash
# List all roosts
roost get

# Get detailed status for our roost
roost status my-first-nginx

# Stream real-time logs
roost logs my-first-nginx -f

# Execute health checks manually
roost health check my-first-nginx

# Get health check history
roost health history my-first-nginx

# Generate configuration templates
roost template generate redis-cache \
  --chart=redis \
  --repo=https://charts.bitnami.com/bitnami \
  --health-type=tcp \
  --teardown-timeout=1h
```

## Step 7: Monitoring and Observability

### Enable Monitoring (if not done during installation)

```bash
# Upgrade Roost-Keeper with monitoring enabled
helm upgrade roost-keeper roost-keeper/roost-keeper \
  --namespace roost-keeper-system \
  --set monitoring.enabled=true \
  --set monitoring.grafana.enabled=true \
  --set monitoring.prometheus.enabled=true
```

### Access Dashboards

```bash
# Port-forward to Grafana (if enabled)
kubectl port-forward svc/roost-keeper-grafana 3000:80 -n roost-keeper-system &

# Visit http://localhost:3000
# Default credentials: admin/admin

# Port-forward to Prometheus
kubectl port-forward svc/roost-keeper-prometheus 9090:9090 -n roost-keeper-system &

# Visit http://localhost:9090
```

### View Metrics

```bash
# Access controller metrics
kubectl port-forward svc/roost-keeper-controller-service 8080:8080 -n roost-keeper-system &

# View available metrics
curl http://localhost:8080/metrics | grep roost_keeper

# Example queries for Prometheus:
# roost_keeper_managedroosts_total
# roost_keeper_health_check_duration_seconds
# roost_keeper_teardown_triggered_total
```

## Step 8: Test Advanced Scenarios

### Simulate Application Failure

```bash
# Break the application to test failure handling
kubectl patch deployment my-first-nginx -p='
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "nginx",
            "image": "nginx:broken-tag"
          }
        ]
      }
    }
  }
}'

# Watch health checks fail
watch "kubectl get managedroost my-first-nginx -o jsonpath='{.status.healthChecks}' | jq '.'"

# Check failure count progression
kubectl get managedroost my-first-nginx -o jsonpath='{.status.healthChecks[0].failureCount}' && echo

# Restore the application
kubectl patch deployment my-first-nginx -p='
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "nginx",
            "image": "nginx:latest"
          }
        ]
      }
    }
  }
}'
```

### Test Teardown Policy

```bash
# Trigger immediate teardown by updating timeout
kubectl patch managedroost my-first-nginx --type='merge' -p='
{
  "spec": {
    "teardownPolicy": {
      "triggers": [
        {
          "type": "timeout",
          "timeout": "1m"
        }
      ]
    }
  }
}'

# Watch teardown process
kubectl get managedroost my-first-nginx -w

# Check teardown status
kubectl get managedroost my-first-nginx -o jsonpath='{.status.teardown}' | jq '.'
```

## Step 9: Explore Configuration Options

### Test Different Chart Sources

```bash
# Create a roost with OCI registry
kubectl apply -f - <<EOF
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: redis-cache
  namespace: default
spec:
  chart:
    name: redis
    repository:
      url: oci://registry-1.docker.io/bitnamicharts
      type: oci
    version: "18.19.4"
    values:
      inline: |
        auth:
          enabled: false
        replica:
          replicaCount: 1
  healthChecks:
    - name: redis-tcp
      type: tcp
      tcp:
        host: redis-cache-master.default.svc.cluster.local
        port: 6379
      interval: 20s
      timeout: 5s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 30m
EOF
```

### Use ConfigMaps for Values

```bash
# Create a ConfigMap with chart values
kubectl create configmap nginx-values --from-literal=values.yaml='
replicaCount: 3
service:
  type: LoadBalancer
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
'

# Use ConfigMap in ManagedRoost
kubectl apply -f - <<EOF
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: nginx-with-configmap
  namespace: default
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
    version: "13.2.23"
    values:
      configMapRefs:
        - name: nginx-values
          key: values.yaml
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://nginx-with-configmap.default.svc.cluster.local"
      interval: 30s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 1h
EOF
```

## Step 10: Cleanup

### Remove Test Roosts

```bash
# Delete individual roosts
kubectl delete managedroost my-first-nginx
kubectl delete managedroost redis-cache
kubectl delete managedroost nginx-with-configmap

# Verify Helm releases are cleaned up
helm list -A

# Clean up ConfigMaps
kubectl delete configmap nginx-values
```

### Optional: Uninstall Roost-Keeper

```bash
# If you want to completely remove Roost-Keeper
helm uninstall roost-keeper -n roost-keeper-system

# Remove namespace
kubectl delete namespace roost-keeper-system

# Remove CRDs (if desired)
kubectl delete crd managedroosts.roost.birb.party
```

## Troubleshooting Common Issues

### Issue 1: Roost Stuck in Pending Phase

**Symptoms:** ManagedRoost remains in `Pending` phase

**Diagnosis:**
```bash
# Check controller logs
kubectl logs -n roost-keeper-system deployment/roost-keeper-controller

# Check events
kubectl get events --sort-by='.lastTimestamp' | grep my-first-nginx

# Check roost status
kubectl describe managedroost my-first-nginx
```

**Common Solutions:**
- Verify Helm repository is accessible
- Check chart name and version exist
- Ensure sufficient cluster resources
- Verify RBAC permissions

### Issue 2: Health Checks Failing

**Symptoms:** Health status shows `unhealthy` or `unknown`

**Diagnosis:**
```bash
# Test connectivity manually
kubectl run test-pod --image=curlimages/curl -i --tty --rm -- sh
# From inside pod: curl http://my-first-nginx.default.svc.cluster.local

# Check service endpoints
kubectl get endpoints my-first-nginx

# Verify service is working
kubectl port-forward svc/my-first-nginx 8080:80 &
curl http://localhost:8080
```

**Common Solutions:**
- Verify service name and port
- Check if pods are ready
- Ensure network policies allow connections
- Adjust health check timeouts

### Issue 3: Teardown Not Working

**Symptoms:** Roost doesn't tear down when expected

**Diagnosis:**
```bash
# Check teardown policy evaluation
kubectl get managedroost my-first-nginx -o jsonpath='{.status.teardown}' | jq '.'

# View controller logs for teardown decisions
kubectl logs -n roost-keeper-system deployment/roost-keeper-controller | grep teardown

# Check if manual approval is required
kubectl get managedroost my-first-nginx -o jsonpath='{.spec.teardownPolicy.requireManualApproval}'
```

**Common Solutions:**
- Verify trigger conditions are met
- Check if manual approval is blocking teardown
- Ensure controller has permissions to delete resources
- Review grace period settings

## Next Steps

Congratulations! You've successfully:
- ✅ Installed Roost-Keeper
- ✅ Created your first ManagedRoost
- ✅ Configured comprehensive health monitoring
- ✅ Set up automatic teardown policies
- ✅ Used CLI tools for management
- ✅ Explored advanced configuration options

### Continue Your Journey:

#### Intermediate Topics
- 📖 [Multi-Environment Deployments](multi-environment.md) - Dev/staging/prod workflows
- 🔍 [Advanced Health Monitoring](advanced-health.md) - Complex health scenarios
- 🔧 [Configuration Reference](../user-guide/configuration.md) - All configuration options

#### Advanced Topics
- 🏢 [Enterprise Integration](enterprise-integration.md) - Production deployment patterns
- 🔒 [Security Configuration](../user-guide/security.md) - RBAC and security hardening
- 📊 [Monitoring Setup](../user-guide/monitoring.md) - Observability and alerting

#### Community & Support
- 💬 [GitHub Discussions](https://github.com/birbparty/roost-keeper/discussions) - Ask questions and share experiences
- 🐛 [Report Issues](https://github.com/birbparty/roost-keeper/issues) - Help improve Roost-Keeper
- 📖 [Best Practices](../examples/) - Learn from real-world examples

## Summary

In this tutorial, you learned the core concepts of Roost-Keeper:

- **ManagedRoosts** - Declarative Helm lifecycle management
- **Health Checks** - Multi-protocol monitoring (HTTP, TCP, Kubernetes)
- **Teardown Policies** - Automated cleanup based on various triggers
- **Observability** - Metrics, logging, and tracing integration
- **CLI Tools** - Enhanced productivity and debugging

Roost-Keeper simplifies managing ephemeral applications in Kubernetes while providing enterprise-grade features for production use. You're now ready to apply these concepts to your own applications and environments!
