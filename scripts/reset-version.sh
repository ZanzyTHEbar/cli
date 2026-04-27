#!/usr/bin/env bash
# Reset version placeholders in internal/version/consts.go and pangolin-cli.wxs
# Usage: ./reset-version.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VERSION_FILE="${REPO_ROOT}/internal/version/consts.go"
INSTALLER_WXS="${REPO_ROOT}/pangolin-cli.wxs"

DEFAULT_CLI_VERSION="version_replaceme"
DEFAULT_INSTALLER_VERSION="0.0.0"

cd "$REPO_ROOT"

if [ ! -f "$VERSION_FILE" ]; then
  echo "Error: $VERSION_FILE not found"
  exit 1
fi

if [ ! -f "$INSTALLER_WXS" ]; then
  echo "Error: $INSTALLER_WXS not found"
  exit 1
fi

echo "Resetting versions to defaults..."
echo ""

echo "Updating $VERSION_FILE..."
case $(uname) in
  Darwin) sed -i '' "s/var Version = \"[^\"]*\"/var Version = \"$DEFAULT_CLI_VERSION\"/" "$VERSION_FILE" ;;
  *)      sed -i    "s/var Version = \"[^\"]*\"/var Version = \"$DEFAULT_CLI_VERSION\"/" "$VERSION_FILE" ;;
esac
if grep -q "var Version = \"$DEFAULT_CLI_VERSION\"" "$VERSION_FILE"; then
  echo "  OK"
else
  echo "Error: failed to reset version in $VERSION_FILE"
  exit 1
fi

echo "Updating pangolin-cli.wxs..."
case $(uname) in
  Darwin) sed -i '' "s/Version=\"[^\"]*\"/Version=\"$DEFAULT_INSTALLER_VERSION\"/" "$INSTALLER_WXS" ;;
  *)      sed -i    "s/Version=\"[^\"]*\"/Version=\"$DEFAULT_INSTALLER_VERSION\"/" "$INSTALLER_WXS" ;;
esac
if grep -q "Version=\"$DEFAULT_INSTALLER_VERSION\"" "$INSTALLER_WXS"; then
  echo "  OK"
else
  echo "Error: failed to reset Version in $INSTALLER_WXS"
  exit 1
fi

echo ""
echo "Versions reset:"
echo "  internal/version/consts.go -> $DEFAULT_CLI_VERSION"
echo "  pangolin-cli.wxs -> $DEFAULT_INSTALLER_VERSION"
