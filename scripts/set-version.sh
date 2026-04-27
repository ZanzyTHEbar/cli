#!/usr/bin/env bash
# Set version in internal/version/consts.go and installer pangolin-cli.wxs
# Usage: ./set-version.sh <version>
#   version: Version string (e.g., 1.0.3)

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <version>"
  echo "  version: Version string (e.g., \"1.0.3\")"
  echo ""
  echo "Example:"
  echo "  $0 1.0.3"
  exit 1
fi

VERSION="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VERSION_FILE="${REPO_ROOT}/internal/version/consts.go"
INSTALLER_WXS="${REPO_ROOT}/pangolin-cli.wxs"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?(-[a-zA-Z0-9]+)?$ ]]; then
  echo "Warning: Version format may be invalid. Expected format: X.Y.Z or X.Y.Z-suffix"
  echo "  Example: 1.0.3 or 1.0.3-beta"
  read -p "Continue anyway? (y/N): " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

cd "$REPO_ROOT"

if [ ! -f "$VERSION_FILE" ]; then
  echo "Error: $VERSION_FILE not found"
  exit 1
fi

if [ ! -f "$INSTALLER_WXS" ]; then
  echo "Error: $INSTALLER_WXS not found"
  exit 1
fi

echo "Setting version to: ${VERSION}"
echo ""

echo "Updating $VERSION_FILE..."
case $(uname) in
  Darwin) sed -i '' "s/var Version = \"[^\"]*\"/var Version = \"$VERSION\"/" "$VERSION_FILE" ;;
  *)      sed -i    "s/var Version = \"[^\"]*\"/var Version = \"$VERSION\"/" "$VERSION_FILE" ;;
esac
if grep -q "var Version = \"$VERSION\"" "$VERSION_FILE"; then
  echo "  OK"
else
  echo "Error: failed to update version in $VERSION_FILE"
  exit 1
fi

echo "Updating pangolin-cli.wxs..."
case $(uname) in
  Darwin) sed -i '' "s/Version=\"[^\"]*\"/Version=\"$VERSION\"/" "$INSTALLER_WXS" ;;
  *)      sed -i    "s/Version=\"[^\"]*\"/Version=\"$VERSION\"/" "$INSTALLER_WXS" ;;
esac
if grep -q "Version=\"$VERSION\"" "$INSTALLER_WXS"; then
  echo "  OK"
else
  echo "Error: failed to update Version in $INSTALLER_WXS"
  exit 1
fi

echo ""
echo "Version updated to $VERSION in internal/version/consts.go and pangolin-cli.wxs"
echo ""
echo "Next: build the Windows binary, then: scripts\\build-msi.bat"
