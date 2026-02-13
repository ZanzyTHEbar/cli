#!/bin/bash

# Get Pangolin - Cross-platform installation script
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/fosrl/cli/refs/heads/main/get-cli.sh | bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# GitHub repository info
REPO="fosrl/cli"
GITHUB_API_URL="https://api.github.com/repos/${REPO}/releases/latest"

# Output helpers
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Fetch latest version from GitHub API
get_latest_version() {
    local latest_info
    if command -v curl >/dev/null 2>&1; then
        latest_info=$(curl -fsSL "$GITHUB_API_URL" 2>/dev/null)
    elif command -v wget >/dev/null 2>&1; then
        latest_info=$(wget -qO- "$GITHUB_API_URL" 2>/dev/null)
    else
        print_error "Neither curl nor wget is available."
        exit 1
    fi

    if [ -z "$latest_info" ]; then
        print_error "Failed to fetch latest version info"
        exit 1
    fi

    local version
    version=$(echo "$latest_info" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    if [ -z "$version" ]; then
        print_error "Could not parse version from GitHub API response"
        exit 1
    fi

    version=$(echo "$version" | sed 's/^v//')
    echo "$version"
}

# Detect OS and architecture
detect_platform() {
    local os arch
    case "$(uname -s)" in
        Linux*) os="linux" ;;
        Darwin*) os="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        FreeBSD*) os="freebsd" ;;
        *) print_error "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        armv7l|armv6l)
            if [ "$os" = "linux" ]; then
                arch="arm32"
            else
                arch="arm64"
            fi
            ;;
        riscv64)
            if [ "$os" = "linux" ]; then
                arch="riscv64"
            else
                print_error "RISC-V only supported on Linux"
                exit 1
            fi
            ;;
        *) print_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

# Determine installation directory
get_install_dir() {
    if [ "$OS" = "windows" ]; then
        echo "$HOME/bin"
    else
        if echo "$PATH" | grep -q "/usr/local/bin" && [ -w "/usr/local/bin" ]; then
            echo "/usr/local/bin"
        else
            echo "$HOME/.local/bin"
        fi
    fi
}

# Download and install Pangolin
install_pangolin() {
    local platform="$1"
    local install_dir="$2"
    local asset_name="pangolin-cli_${platform}"
    local exe_suffix=""
    local final_name="pangolin"

    if [[ "$platform" == *"windows"* ]]; then
        asset_name="${asset_name}.exe"
        exe_suffix=".exe"
        final_name="pangolin.exe"
    fi

    local download_url="${BASE_URL}/${asset_name}"
    local temp_file="/tmp/${final_name}"
    local final_path="${install_dir}/${final_name}"

    print_status "Downloading Pangolin from ${download_url}"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$download_url" -o "$temp_file"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$download_url" -O "$temp_file"
    else
        print_error "Neither curl nor wget is available."
        exit 1
    fi

    mkdir -p "$install_dir"
    mv "$temp_file" "$final_path"
    chmod +x "$final_path"

    print_status "Pangolin installed to ${final_path}"

    if ! echo "$PATH" | grep -q "$install_dir"; then
        print_warning "Install directory ${install_dir} is not in your PATH."
        print_warning "Add it with:"
        print_warning "  export PATH=\"${install_dir}:\$PATH\""
    fi
}

# Verify installation
verify_installation() {
    local install_dir="$1"
    local exe_suffix=""

    if [[ "$PLATFORM" == *"windows"* ]]; then
        exe_suffix=".exe"
    fi

    local pangolin_path="${install_dir}/pangolin${exe_suffix}"
    if [ -x "$pangolin_path" ]; then
        print_status "Installation successful!"
        print_status "pangolin version: $("$pangolin_path" version 2>/dev/null || echo "unknown")"
        return 0
    else
        print_error "Installation failed. Binary not found or not executable."
        return 1
    fi
}

# Main function
main() {
    print_status "Installing latest version of Pangolin..."

    print_status "Fetching latest version..."
    VERSION=$(get_latest_version)
    print_status "Latest version: v${VERSION}"

    BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

    PLATFORM=$(detect_platform)
    print_status "Detected platform: ${PLATFORM}"

    INSTALL_DIR=$(get_install_dir)
    print_status "Install directory: ${INSTALL_DIR}"

    install_pangolin "$PLATFORM" "$INSTALL_DIR"

    if verify_installation "$INSTALL_DIR"; then
        print_status "Pangolin is ready to use!"
        print_status "Run 'pangolin --help' to get started."
    else
        exit 1
    fi
}

main "$@"
