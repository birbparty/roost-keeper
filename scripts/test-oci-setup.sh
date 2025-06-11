#!/bin/bash

# Test script for OCI registry setup and chart upload
# This script helps set up test charts in the local Zot registry

set -e

REGISTRY="10.0.0.106:30001"
CHART_NAME="nginx"
CHART_VERSION="1.0.0"

echo "🚀 Setting up OCI registry test environment..."

# Check if registry is accessible
echo "📡 Checking registry connectivity..."
if curl -f "http://${REGISTRY}/v2/" > /dev/null 2>&1; then
    echo "✅ Registry is accessible at ${REGISTRY}"
else
    echo "❌ Registry not accessible at ${REGISTRY}"
    echo "💡 Make sure the Zot registry is running:"
    echo "   - Check: kubectl get pods -A | grep zot"
    echo "   - Or visit: http://${REGISTRY} in your browser"
    exit 1
fi

# Check if helm is installed
if ! command -v helm &> /dev/null; then
    echo "❌ Helm CLI not found. Please install Helm:"
    echo "   brew install helm"
    exit 1
fi

# Check if oras is installed
if ! command -v oras &> /dev/null; then
    echo "❌ ORAS CLI not found. Please install ORAS:"
    echo "   brew install oras"
    exit 1
fi

echo "✅ Required tools are available"

# Create a test chart
echo "📦 Creating test Helm chart..."
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"

# Create a simple nginx chart
helm create "$CHART_NAME"

# Customize the chart version
sed -i '' "s/version: 0.1.0/version: ${CHART_VERSION}/" "${CHART_NAME}/Chart.yaml"

echo "📝 Chart.yaml contents:"
cat "${CHART_NAME}/Chart.yaml"

# Package the chart
echo "📦 Packaging chart..."
helm package "$CHART_NAME"

CHART_FILE="${CHART_NAME}-${CHART_VERSION}.tgz"

if [[ ! -f "$CHART_FILE" ]]; then
    echo "❌ Chart packaging failed"
    exit 1
fi

echo "✅ Chart packaged: $CHART_FILE"

# Upload to OCI registry using ORAS
echo "🔄 Uploading chart to OCI registry..."
CHART_REPO="oci://${REGISTRY}/${CHART_NAME}"

echo "📤 Pushing to: ${CHART_REPO}:${CHART_VERSION}"

# Use ORAS to push (with --insecure for local development)
if oras push "${CHART_REPO}:${CHART_VERSION}" "$CHART_FILE" --insecure; then
    echo "✅ Chart uploaded successfully!"
else
    echo "❌ Chart upload failed"
    echo "💡 Try manually:"
    echo "   cd $TEMP_DIR"
    echo "   oras push ${CHART_REPO}:${CHART_VERSION} $CHART_FILE --insecure"
    exit 1
fi

# Verify the upload
echo "🔍 Verifying chart upload..."
if oras manifest fetch "${CHART_REPO}:${CHART_VERSION}" --insecure > /dev/null 2>&1; then
    echo "✅ Chart verification successful!"
else
    echo "⚠️  Chart verification failed, but upload may have succeeded"
fi

# List repository contents
echo "📋 Repository contents:"
curl -s "http://${REGISTRY}/v2/_catalog" | jq '.' || echo "Unable to fetch catalog"

echo ""
echo "🎉 OCI test setup complete!"
echo ""
echo "🧪 Now you can test the Roost-Keeper OCI integration:"
echo "   ROOST_KEEPER_INTEGRATION_TEST=true go test ./test/integration/... -run TestOCIRegistryIntegration -v"
echo ""
echo "📝 Test chart details:"
echo "   Registry: oci://${REGISTRY}"
echo "   Chart: ${CHART_NAME}"
echo "   Version: ${CHART_VERSION}"
echo "   Full reference: oci://${REGISTRY}/${CHART_NAME}:${CHART_VERSION}"
echo ""
echo "🧹 Cleanup:"
echo "   rm -rf $TEMP_DIR"

# Cleanup
cd /
rm -rf "$TEMP_DIR"
