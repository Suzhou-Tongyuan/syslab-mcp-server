#!/usr/bin/env sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR=${OUT_DIR:-"$ROOT_DIR/bin"}
GO_CACHE=${GOCACHE:-"$ROOT_DIR/.cache/go-build"}
VERSION_FILE="$ROOT_DIR/VERSION"
APP_DIR="$ROOT_DIR/cmd/syslab-mcp-core-server"

resolve_version() {
    if [ -n "${VERSION:-}" ]; then
        printf '%s\n' "$VERSION"
        return
    fi

    if [ -n "${CI_COMMIT_TAG:-}" ]; then
        printf '%s\n' "$CI_COMMIT_TAG"
        return
    fi

    if [ -f "$VERSION_FILE" ]; then
        tr -d '\r\n' <"$VERSION_FILE"
        printf '\n'
        return
    fi

    printf '0.1.0\n'
}

build_target() {
    goos=$1
    goarch=$2
    outfile=$3

    echo "Building $goos/$goarch -> $outfile"
    GOOS=$goos GOARCH=$goarch go build \
        -trimpath \
        -ldflags "-s -w -X main.version=$BUILD_VERSION" \
        -o "$OUT_DIR/$outfile" \
        "$APP_DIR"
}

BUILD_VERSION=$(resolve_version)

mkdir -p "$OUT_DIR" "$GO_CACHE"

export GOCACHE="$GO_CACHE"
export CGO_ENABLED="${CGO_ENABLED:-0}"

build_target windows amd64 syslab-mcp-server-win64.exe
build_target windows arm64 syslab-mcp-server-winarm64.exe
build_target linux amd64 syslab-mcp-server-glnxa64
build_target linux arm64 syslab-mcp-server-glnxarm64
