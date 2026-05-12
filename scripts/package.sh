#!/usr/bin/env sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN_DIR=${BIN_DIR:-"$ROOT_DIR/bin"}

hash_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1"
        return
    fi

    shasum -a 256 "$1"
}

create_bundle() {
    zip_name=$1
    binary_name=$2

    rm -f "$BIN_DIR/$zip_name"
    go run "$ROOT_DIR/cmd/release-zip" \
        "$BIN_DIR/$zip_name" \
        "$ROOT_DIR/README.md:README.md" \
        "$BIN_DIR/$binary_name:$binary_name"
}

prepare_binary() {
    binary_name=$1

    case "$binary_name" in
        *.exe) ;;
        *) chmod 755 "$BIN_DIR/$binary_name" ;;
    esac
    echo "Prepared $BIN_DIR/$binary_name"
}

if [ "${SKIP_BUILD:-0}" != "1" ]; then
    sh "$ROOT_DIR/scripts/build.sh"
fi

mkdir -p "$BIN_DIR"
rm -f \
    "$BIN_DIR/SHA256SUMS" \
    "$BIN_DIR/syslab-mcp-server-win64.zip" \
    "$BIN_DIR/syslab-mcp-server-glnxa64.zip"

prepare_binary syslab-mcp-server-win64.exe
prepare_binary syslab-mcp-server-winarm64.exe
prepare_binary syslab-mcp-server-glnxa64
prepare_binary syslab-mcp-server-glnxarm64
create_bundle syslab-mcp-server-win64.zip syslab-mcp-server-win64.exe
create_bundle syslab-mcp-server-glnxa64.zip syslab-mcp-server-glnxa64

(
    cd "$BIN_DIR"
    : >SHA256SUMS
    for asset in *; do
        [ -f "$asset" ] || continue
        [ "$asset" = "SHA256SUMS" ] && continue
        hash_file "$asset" >>SHA256SUMS
    done
)
