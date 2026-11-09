#!/bin/sh
# Builds a throwaway world for the demo recording: agentctl, a fake
# CLIProxyAPI called "home" and an unreachable one called "spark", and agent
# configs in a scratch home. Nothing touches your real config. The home is a
# short path because the recording shows it.
# Usage: . docs/demo/setup.sh <dir>
set -eu
repo=$(git rev-parse --show-toplevel)
rm -rf "$1"
mkdir -p "$1"
demo=$(cd "$1" && pwd)
mkdir -p "$demo/.local/bin" "$demo/.codex" "$demo/.config/agentctl" "$demo/.config/opencode" "$demo/.pi/agent"
(cd "$repo" && go build -o "$demo/.local/bin/agentctl" ./cmd/agentctl)

cat > "$demo/.config/agentctl/config.toml" <<'TOML'
[proxies.home]
url = 'http://127.0.0.1:18317'
key = 'sk-demo'

[proxies.spark]
url = 'http://127.0.0.1:18445'
key = 'sk-demo'
TOML
echo '{"claude_proxy_acknowledged": true}' > "$demo/.config/agentctl/state.json"
printf 'model = "gpt-6.1-sol"\n' > "$demo/.codex/config.toml"
printf '{\n  "$schema": "https://opencode.ai/config.json"\n}\n' > "$demo/.config/opencode/opencode.json"
printf '{\n  "defaultProvider": "openai-codex",\n  "defaultModel": "gpt-6.1-sol"\n}\n' > "$demo/.pi/agent/settings.json"

python3 -I "$repo/docs/demo/fakeproxy.py" 18317 &
echo $! > "$demo/proxy.pid"

cat > "$demo/env.sh" <<ENV
export HOME=$demo PATH=$demo/.local/bin:\$PATH
export XDG_CONFIG_HOME=$demo/.config CLAUDE_CONFIG_DIR=$demo/.claude CODEX_HOME=$demo/.codex
export PI_CODING_AGENT_DIR=$demo/.pi/agent T3CODE_HOME=$demo/.t3
unset NO_COLOR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY OPENCODE_CONFIG_DIR
export PS1='\$ '
ENV
