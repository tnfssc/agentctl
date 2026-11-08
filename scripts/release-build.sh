#!/bin/sh
# Cross-build the supported release binaries. Run through mise.
set -eu

version=${1:?usage: release-build.sh vX.Y.Z}
commit=$(git rev-parse --short HEAD)
mkdir -p dist
for os in darwin linux; do
    for arch in amd64 arm64; do
        echo "Building agentctl $version ($os/$arch)"
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
            -ldflags "-s -w -X github.com/tnfssc/agentctl/internal/cli.version=$version -X github.com/tnfssc/agentctl/internal/cli.commit=$commit" \
            -o "dist/agentctl-$os-$arch" ./cmd/agentctl
    done
done
(
    cd dist
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum agentctl-darwin-amd64 agentctl-darwin-arm64 agentctl-linux-amd64 agentctl-linux-arm64 > SHA256SUMS
    else
        shasum -a 256 agentctl-darwin-amd64 agentctl-darwin-arm64 agentctl-linux-amd64 agentctl-linux-arm64 > SHA256SUMS
    fi
)
