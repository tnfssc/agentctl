---
name: tagged-releases
description: Tagged release and installer convention copied from the owner's other Go repos
---

First release: v0.1.0. Repo's default branch is develop (renamed from main).

- Follow tnfssc/skillz and ghstats: plain Go cross-builds, raw binaries named agentctl-<os>-<arch>, SHA256SUMS,
  softprops/action-gh-release. No GoReleaser, UPX, registries or extra release secrets.
- Supported release platforms: macOS/Linux × amd64/arm64, CGO_ENABLED=0. Windows isn't supported by the config/key
  command setup, so don't publish Windows binaries just because a cross-build succeeds.
- Pushes to develop and PRs run mise run check. v* tags reuse those checks before building and publishing.
- install.sh is attached to releases; the README downloads it from releases/latest. It selects platform, checks SHA-256
  and atomically replaces ~/.local/bin/agentctl, with no sudo. AGENTCTL_VERSION and AGENTCTL_INSTALL_DIR override defaults.
- scripts/release-build.sh injects internal/cli.version and commit. Local equivalent: mise run release-build vX.Y.Z.
- README GIFs (`docs/demo/*.tape`, `mise run demo` into `.tmp/demo/`) are never committed: they are attached to the
  release (`gh release upload <tag> .tmp/demo/*.gif`) and the README links `releases/download/<tag>/<name>.gif`.
  One per feature since v1.6.0 (user, 2026-11-12): demo (routing), cliproxy, t3, opencode, tune, phone. The demo world
  (`docs/demo/setup.sh`) fakes T3 Code, OpenCode and launchctl; the cliproxy tape downloads the real release.
