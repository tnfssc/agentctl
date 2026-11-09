#!/bin/sh
# Install an agentctl release for macOS or Linux; no root access needed.
set -eu

fail() { printf 'agentctl: %s\n' "$*" >&2; exit 1; }

for tool in curl mktemp; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done

case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail 'supported systems: macOS and Linux' ;;
esac
case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) fail 'supported architectures: amd64 and arm64' ;;
esac

repo=https://github.com/tnfssc/agentctl
version=${AGENTCTL_VERSION:-latest}
asset="agentctl-${os}-${arch}"
if [ "$version" = latest ]; then
    base="$repo/releases/latest/download"
else
    printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$' \
        || fail "invalid release version: $version"
    base="$repo/releases/download/$version"
fi
work=$(mktemp -d "${TMPDIR:-/tmp}/agentctl-install.XXXXXX")
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM

printf 'Downloading agentctl %s (%s/%s)…\n' "$version" "$os" "$arch"
curl -fsSL --connect-timeout 15 --max-time 180 --retry 3 "$base/$asset" -o "$work/$asset"
curl -fsSL --connect-timeout 15 --max-time 60 --retry 3 "$base/SHA256SUMS" -o "$work/SHA256SUMS"
expected=$(awk -v file="$asset" '$2 == file { print $1 }' "$work/SHA256SUMS")
[ "${#expected}" -eq 64 ] || fail "missing checksum for $asset"
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$work/$asset" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$work/$asset" | awk '{ print $1 }')
else
    fail 'sha256sum or shasum is required'
fi
[ "$actual" = "$expected" ] || fail 'checksum mismatch; nothing installed'

install_dir=${AGENTCTL_INSTALL_DIR:-"$HOME/.local/bin"}
mkdir -p "$install_dir"
# Stage beside the destination so rename is atomic, even across filesystems.
staged=$(mktemp "$install_dir/.agentctl.XXXXXX")
trap 'rm -rf "$work"; rm -f "$staged"' EXIT
cat "$work/$asset" > "$staged"
chmod 755 "$staged"
mv -f "$staged" "$install_dir/agentctl"
cat <<'LOGO'

┌─┐┌─┐┌─┐┌┐┌┌┬┐┌─┐┌┬┐┬
├─┤│ ┬├┤ │││ │ │   │ │
┴ ┴└─┘└─┘┘└┘ ┴ └─┘ ┴ ┴─┘

LOGO
printf 'Installed %s/agentctl\n' "$install_dir"
case ":$PATH:" in
    *":$install_dir:"*) printf 'Run: agentctl\n' ;;
    *) printf 'Add %s to your PATH, or run: %s/agentctl\n' "$install_dir" "$install_dir" ;;
esac
