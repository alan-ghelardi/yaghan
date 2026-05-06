#!/usr/bin/env bash

set -euo pipefail

# Generate REST API documentation from OpenAPI/Swagger files

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEN_DIR="$(realpath $SCRIPT_DIR/../gen)"
DOCS_DIR="$GEN_DIR/docs/rest"

# Create docs directory if it doesn't exist
mkdir -p "$DOCS_DIR"

# Check if widdershins is installed, install if not
if ! command -v widdershins &> /dev/null; then
    echo "widdershins not found. Installing..."

    # Check if npm is available
    if ! command -v npm &> /dev/null; then
        echo "Error: npm is not installed. Please install Node.js and npm first."
        echo "Visit: https://nodejs.org/"
        exit 1
    fi

    # Install widdershins globally
    echo "Installing widdershins via npm..."
    npm install -g widdershins

    # Verify installation
    if ! command -v widdershins &> /dev/null; then
        echo "Error: Failed to install widdershins"
        exit 1
    fi

    echo "✓ widdershins installed successfully"
    echo ""
fi

echo "Generating REST API documentation from OpenAPI files..."

# Function to convert OpenAPI to Markdown
generate_markdown() {
    local swagger_file="$1"
    local output_file="$2"
    local service_name="$3"

    echo "  → Converting $service_name..."

    widdershins "$swagger_file" \
                -o "$output_file" \
                --language_tabs 'shell:cURL' 'python:Python' 'javascript:JavaScript' 'go:Go' \
                --summary \
                --omitHeader false \
                --code true \
                2>/dev/null

    if [ $? -eq 0 ]; then
        echo "    ✓ Generated $output_file"
    else
        echo "    ✗ Failed to generate $output_file"
        return 1
    fi
}

# Generate documentation for each service
generate_markdown \
    "$GEN_DIR/nuinfra/control_plane/v1alpha1/cluster.swagger.json" \
    "$DOCS_DIR/cluster-api.md" \
    "Cluster Service"

generate_markdown \
    "$GEN_DIR/nuinfra/control_plane/v1alpha1/sandbox.swagger.json" \
    "$DOCS_DIR/sandbox-api.md" \
    "Sandbox Service"
