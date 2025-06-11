# Zot OCI Registry Deployment Request

## Purpose
Deploy a Zot OCI registry on the local Kubernetes cluster for testing Helm SDK OCI integration in Roost-Keeper.

## Background
The Roost-Keeper project needs to test OCI (Open Container Initiative) registry functionality for Helm chart operations. We require a dedicated test environment to validate:
- OCI chart push/pull operations
- Authentication mechanisms
- Integration with Digital Ocean Container Registry patterns
- Performance characteristics

## Requirements

### Deployment Target
- **Cluster**: Local Kubernetes cluster at `10.0.0.106`
- **Namespace**: `zot-registry` (or similar)
- **Service Type**: LoadBalancer or NodePort for external access

### Zot Registry Configuration
```yaml
# Suggested Zot configuration
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zot-registry
  namespace: zot-registry
spec:
  replicas: 1
  selector:
    matchLabels:
      app: zot-registry
  template:
    metadata:
      labels:
        app: zot-registry
    spec:
      containers:
      - name: zot
        image: ghcr.io/project-zot/zot:latest
        ports:
        - containerPort: 5000
        env:
        - name: ZOT_LOG_LEVEL
          value: "debug"
        volumeMounts:
        - name: config
          mountPath: /etc/zot
        - name: data
          mountPath: /var/lib/registry
      volumes:
      - name: config
        configMap:
          name: zot-config
      - name: data
        emptyDir: {}
```

### Authentication Requirements
- **Basic Authentication**: Username/password support
- **Test Credentials**: 
  - Username: `roost-test`
  - Password: `roost-test-password` (or generate secure password)
- **Anonymous Read**: Optional for public test charts

### Service Exposure
```yaml
apiVersion: v1
kind: Service
metadata:
  name: zot-registry-service
  namespace: zot-registry
spec:
  type: LoadBalancer  # or NodePort
  ports:
  - port: 5000
    targetPort: 5000
    protocol: TCP
  selector:
    app: zot-registry
```

### Storage Requirements
- **Persistent Volume**: 10Gi for chart storage
- **Storage Class**: Default or fast SSD for performance testing

## Test Data Setup

### Sample Test Charts
Please create or ensure the following test charts are available:
1. **nginx-test**: Simple nginx deployment chart
2. **redis-test**: Basic redis chart for dependency testing
3. **multi-chart**: Chart with multiple dependencies

### Chart Repository Structure
```
registry-endpoint/
├── roost-test/
│   ├── nginx-test:1.0.0
│   ├── nginx-test:1.1.0
│   ├── redis-test:1.0.0
│   └── multi-chart:1.0.0
```

## Expected Deliverables

### Registry Access Information
- **Registry URL**: `http://10.0.0.106:<port>` or custom hostname
- **Authentication Endpoint**: For login testing
- **Health Check Endpoint**: `/v2/` for registry health

### Test Credentials
- **Username/Password**: For authenticated operations
- **Test Token**: If token-based auth is configured
- **Repository Names**: Available test repositories

### Documentation
- **Access Instructions**: How to connect and authenticate
- **Chart Upload Process**: How to push test charts
- **Troubleshooting**: Common issues and solutions

## Testing Validation

### Basic Functionality Tests
```bash
# Registry health check
curl -f http://registry-url/v2/

# Authentication test
helm registry login registry-url -u roost-test -p roost-test-password

# Chart operations
helm push chart.tgz oci://registry-url/roost-test
helm pull oci://registry-url/roost-test/nginx-test --version 1.0.0
```

### Integration Test Scenarios
1. **Chart Push/Pull**: Basic OCI operations
2. **Authentication**: Username/password and token-based
3. **Helm Install**: Install charts directly from OCI registry
4. **Error Handling**: Network failures, auth failures, missing charts

## Timeline
- **Requested By**: [Date]
- **Required By**: [Date + 3-5 business days]
- **Testing Window**: [Date range for testing activities]

## Contact Information
- **Project Lead**: [Your Name]
- **Development Team**: Roost-Keeper team
- **Use Case**: Helm SDK OCI integration testing

## Additional Notes
- This is a **test environment only** - no production data
- Registry can be ephemeral (data loss acceptable)
- Performance testing may generate moderate load
- Clean up after testing completion if requested

---

**Priority**: Medium-High (blocks OCI integration development)
**Impact**: Required for Helm SDK OCI functionality completion
