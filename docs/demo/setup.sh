#!/bin/sh
# Builds a throwaway world for the demo recordings: agentctl, a fake
# CLIProxyAPI called "home" and an unreachable one called "spark", agent
# configs, fake T3 Code and OpenCode servers, and a stand-in launchctl that
# runs a real CLIProxyAPI install in the background, all in a scratch home.
# Nothing touches your real config or services. The home is a short path
# because the recordings show it.
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
printf '{\n  "defaultProvider": "openai",\n  "defaultModel": "o3"\n}\n' > "$demo/.pi/agent/settings.json"

python3 -I "$repo/docs/demo/fakeproxy.py" 18317 &
echo $! > "$demo/proxy.pid"

# T3 Code: a service runtime whose t3 answers from canned JSON; its server is
# the fake proxy above.
t3v=0.1.8-nightly.20261111.4420
t3=$demo/.t3
mkdir -p "$t3/runtime/versions/$t3v" "$t3/userdata" "$demo/t3"
echo '{"protocol":3,"activeVersion":"'$t3v'"}' > "$t3/runtime/service-state.json"
echo '{"version":1,"pid":1,"origin":"http://127.0.0.1:18317","serviceManaged":true}' > "$t3/userdata/server-runtime.json"
python3 -I - "$demo/t3" <<'PY'
import datetime as d, json, sys
now = d.datetime.now(d.timezone.utc)
iso = lambda delta: (now - delta).strftime("%Y-%m-%dT%H:%M:%SZ")
h, day = d.timedelta(hours=1), d.timedelta(days=1)
def s(sid, label, os, browser, connected):
    return {"sessionId": sid, "subject": "one-time-token", "client": {"label": label, "os": os, "browser": browser},
            "issuedAt": iso(20 * day), "expiresAt": iso(-10 * day), "lastConnectedAt": iso(connected)}
sessions = [s("a1", "phone", "iOS", "Safari", 2 * h), s("b2", "laptop", "macOS", "Chrome", 26 * h),
            s("c3", "old tablet", "Android", "Chrome", 12 * day)]
open(sys.argv[1] + "/sessions.json", "w").write(json.dumps(sessions))
open(sys.argv[1] + "/links.json", "w").write("[]")
PY
cat > "$t3/runtime/versions/$t3v/t3" <<SH
#!/bin/sh
[ "\$*" = --version ] && { echo "t3 v$t3v"; exit 0; }
case "\$*" in
"status --json") echo '{"service":{"supported":true,"installed":true,"installedVersion":"$t3v"}}' ;;
"auth pairing list --json") cat $demo/t3/links.json ;;
"auth session list --json") cat $demo/t3/sessions.json ;;
"auth pairing create"*)
  base=""; while [ \$# -gt 0 ]; do [ "\$1" = --base-url ] && base="\$2"; shift; done
  exp=\$(date -u -v+5M +%Y-%m-%dT%H:%M:%SZ)
  printf '{"id":"new","credential":"Kq7Xw2Pn4Rt9","expiresAt":"%s","pairUrl":"%s/pair#token=Kq7Xw2Pn4Rt9"}\n' "\$exp" "\$base" ;;
*) echo ok ;;
esac
SH
chmod +x "$t3/runtime/versions/$t3v/t3"

# OpenCode v2: its background server is a file holding the URL while it runs.
mkdir -p "$demo/.local/state/opencode" "$demo/opencode"
echo "http://127.0.0.1:49374" > "$demo/opencode/url"
echo '{"version":"2.2.7","url":"http://127.0.0.1:49374","pid":1}' > "$demo/.local/state/opencode/service.json"
cat > "$demo/.local/bin/opencode" <<SH
#!/bin/sh
url=\$(cat $demo/opencode/url)
case "\$*" in
--version) echo "opencode v2.2.7" ;;
"service status") echo "\${url:-stopped}" ;;
"service start"|"service restart") echo http://127.0.0.1:49374 > $demo/opencode/url; cat $demo/opencode/url ;;
"service stop"|"service unset password") : > $demo/opencode/url ;;
"auth list") echo "OpenAI   OAuth   stored" ;;
"pair --url "*) printf '\n  Open a link to connect. Links work once and expire in 5 minutes.\n\n  %s/auth/connect/Hx81Wq0bZr5LmT2vYd9Kc\n\n' "\$3" ;;
*) echo ok ;;
esac
SH
chmod +x "$demo/.local/bin/opencode"

# launchctl for agentctl's CLIProxyAPI service: runs the installed server in
# the background instead of loading a launchd agent.
cpa=$demo/.local/share/agentctl/cliproxyapi
cat > "$demo/.local/bin/launchctl" <<SH
#!/bin/sh
pid=$demo/cliproxy.pid
alive() { [ -f \$pid ] && kill -0 \$(cat \$pid) 2>/dev/null; }
start() { (cd $cpa && exec ./cli-proxy-api -config $cpa/config.yaml) >> $cpa/cliproxyapi.log 2>&1 < /dev/null & echo \$! > \$pid; }
case "\$1" in
print) alive ;;
bootstrap) alive || start ;;
kickstart) [ "\$2" = -k ] && alive && { kill \$(cat \$pid); sleep 1; }; alive || start ;;
bootout) alive && kill \$(cat \$pid); rm -f \$pid ;;
esac
exit \$?
SH
chmod +x "$demo/.local/bin/launchctl"

cat > "$demo/env.sh" <<ENV
export HOME=$demo PATH=$demo/.local/bin:\$PATH
export XDG_CONFIG_HOME=$demo/.config CLAUDE_CONFIG_DIR=$demo/.claude CODEX_HOME=$demo/.codex
export PI_CODING_AGENT_DIR=$demo/.pi/agent T3CODE_HOME=$demo/.t3 XDG_STATE_HOME=$demo/.local/state
export XDG_DATA_HOME=$demo/.local/share
unset NO_COLOR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY OPENCODE_CONFIG_DIR
export PS1='\$ '
ENV
