# 🚀 Roost CLI Tool Implementation

## Overview

The roost CLI tool provides a comprehensive command-line interface for managing ManagedRoost resources in Kubernetes. Built using the urfave/cli framework (as requested), it offers both standalone binary functionality and kubectl plugin integration.

## Architecture

### Core Components

1. **Main Entry Point** (`cmd/roost/main.go`)
   - urfave/cli framework integration
   - Global configuration and flags
   - kubectl plugin mode detection
   - Context-aware initialization

2. **Configuration Management** (`internal/cli/config.go`)
   - Smart Kubernetes client initialization
   - Telemetry integration
   - Context and namespace detection
   - Lazy loading for non-Kubernetes commands

3. **Command Implementation**
   - **CRUD Operations** (`internal/cli/commands.go`)
   - **Status Monitoring** (`internal/cli/status.go`)
   - **Log Streaming** (`internal/cli/logs.go`)
   - **Health Checks** (`internal/cli/health.go`)
   - **Template Generation** (`internal/cli/template.go`)
   - **Shell Completion** (`internal/cli/completion.go`)
   - **Version Information** (`internal/cli/version.go`)

## Command Structure

```
roost
├── get              # List/get ManagedRoost resources
├── create           # Create resources from YAML files
├── apply            # Apply/update resources
├── delete           # Delete resources with confirmation
├── describe         # Show detailed resource information
├── status           # Real-time status monitoring
├── logs             # Stream logs from associated pods
├── health           # Health check operations
│   ├── check        # Execute health checks
│   ├── list         # List health checks
│   └── debug        # Debug health check issues
├── template         # Template generation
│   ├── generate     # Generate YAML templates
│   └── validate     # Validate templates
├── completion       # Shell completion scripts
│   ├── bash         # Bash completion
│   ├── zsh          # Zsh completion
│   ├── fish         # Fish completion
│   └── powershell   # PowerShell completion
└── version          # Version information
```

## Key Features

### 1. **Multiple Operating Modes**
- **Standalone Binary**: `roost` command with full functionality
- **kubectl Plugin**: `kubectl-roost` for native kubectl integration

### 2. **Comprehensive Resource Operations**
- **CRUD Operations**: Complete lifecycle management
- **Multiple Output Formats**: table, yaml, json, wide
- **Label Selectors**: Advanced resource filtering
- **Namespace Support**: All-namespaces and context-aware operations

### 3. **Advanced Monitoring**
- **Real-time Status**: Watch mode with auto-refresh
- **Health Check Integration**: Execute, list, and debug health checks
- **Log Streaming**: Multi-pod log aggregation with follow mode
- **Detailed Diagnostics**: Comprehensive resource inspection

### 4. **Template System**
- **Smart Presets**: 
  - `simple` - Basic roost configuration
  - `complex` - Full-featured with multiple health checks
  - `monitoring` - Prometheus-focused setup
  - `webapp` - Web application optimized
- **Template Validation**: YAML syntax and schema validation
- **Customizable Output**: File or stdout generation

### 5. **Shell Integration**
- **Comprehensive Completion**: bash, zsh, fish, PowerShell
- **Context-aware Suggestions**: ManagedRoost names, namespaces, contexts
- **Flag Completion**: All command flags and options

### 6. **User Experience**
- **Colored Output**: Status indicators with emojis
- **Progress Feedback**: Real-time operation status
- **Error Handling**: Clear error messages and suggestions
- **Help System**: Comprehensive help and usage information

## Usage Examples

### Basic Operations
```bash
# List all roosts
roost get

# Get specific roost with detailed output
roost get my-app -o wide

# Create from file
roost create -f roost.yaml

# Apply configuration
roost apply -f roost.yaml

# Delete with confirmation
roost delete my-app
```

### Advanced Monitoring
```bash
# Watch status in real-time
roost status my-app --watch

# Stream logs with follow
roost logs my-app --follow

# Debug health checks
roost health debug my-app --check-name http-health

# Execute specific health check
roost health check my-app --check-name prometheus-query
```

### Template Generation
```bash
# Generate simple template
roost template generate my-app --preset simple

# Generate complex monitoring setup
roost template generate prometheus --preset monitoring \
  --chart prometheus --repo https://prometheus-community.github.io/helm-charts

# Generate and save to file
roost template generate my-webapp --preset webapp --output webapp.yaml

# Validate template
roost template validate -f webapp.yaml
```

### Shell Completion
```bash
# Bash completion
source <(roost completion bash)

# Zsh completion
roost completion zsh > _roost

# Fish completion
roost completion fish | source
```

## Global Flags

| Flag | Alias | Description | Environment Variable |
|------|-------|-------------|---------------------|
| `--kubeconfig` | | Path to kubeconfig file | `KUBECONFIG` |
| `--context` | | Kubernetes context to use | `KUBECTL_CONTEXT` |
| `--namespace` | `-n` | Kubernetes namespace | `KUBECTL_NAMESPACE` |
| `--output` | `-o` | Output format (table, yaml, json, wide) | |
| `--verbose` | | Enable verbose output | |
| `--debug` | | Enable debug logging | `ROOST_DEBUG` |

## kubectl Plugin Support

The CLI automatically detects when running as a kubectl plugin:

```bash
# Install as kubectl plugin
cp bin/roost /usr/local/bin/kubectl-roost

# Use as kubectl plugin
kubectl roost get
kubectl roost status my-app --watch
kubectl roost health check my-app
```

## Integration Points

### Health Check System
- Direct integration with all health check engines
- Support for HTTP, TCP, gRPC, Prometheus, and Kubernetes checks
- Real-time execution and debugging capabilities

### Telemetry Integration
- Structured logging with correlation IDs
- Operation tracking and metrics
- Error reporting and diagnostics

### Event System
- Real-time status updates
- Event-driven monitoring
- Lifecycle tracking

## Build and Installation

### Building from Source
```bash
# Build standalone binary
go build -o bin/roost ./cmd/roost

# Build with version information
go build -ldflags="-X main.Version=v1.0.0 -X main.GitCommit=$(git rev-parse HEAD) -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" -o bin/roost ./cmd/roost
```

### Installation as kubectl Plugin
```bash
# Copy to PATH for kubectl plugin support
cp bin/roost /usr/local/bin/kubectl-roost
chmod +x /usr/local/bin/kubectl-roost
```

## Technical Implementation Details

### Smart Client Initialization
- Kubernetes clients are only initialized for commands that need them
- Version, completion, and template commands work without Kubernetes access
- Lazy loading improves startup performance

### Error Handling
- Comprehensive error messages with suggestions
- Graceful handling of network and authentication issues
- Fallback mechanisms for degraded functionality

### Performance Optimizations
- Efficient resource caching
- Minimal API calls
- Smart watch implementations

## Future Enhancements

### Planned Features
- Interactive mode for guided resource creation
- Configuration profiles and contexts
- Advanced filtering and sorting options
- Export/import functionality
- Plugin system for custom commands

### Integration Opportunities
- CI/CD pipeline integration
- GitOps workflow support
- Monitoring system connectors
- Custom dashboard generation

## Conclusion

The roost CLI tool provides a production-ready, comprehensive interface for managing ManagedRoost resources. It follows kubectl patterns and conventions while providing enhanced functionality specific to the Roost-Keeper ecosystem. The implementation is scalable, maintainable, and provides an excellent developer and operator experience.

The CLI successfully transforms complex Kubernetes operations into simple, memorable commands, making the Roost-Keeper system accessible and productive for all users.
