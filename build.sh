#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> Syncing skills to workspace..."
cp -r agent/skills .simpleclaw/workspace/skills
echo "    Copied agent/skills/ -> .simpleclaw/workspace/skills/"

echo "==> Generating goscript package bindings..."
cd goscript/packages/tool && go run tool.go
cd - > /dev/null

echo "==> Building simpleclaw..."
go build -o simpleclaw ./cmd/

echo "==> Done: ./simpleclaw"
