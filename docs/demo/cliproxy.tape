# Records .tmp/demo/cliproxy.gif: a CLIProxyAPI server on this machine: install, keys, logins. Run `mise run demo`, which sets up
# the scratch world this expects in /tmp/agentctl-demo.
Output .tmp/demo/cliproxy.gif

Set Shell "bash"
Set FontSize 15
Set Width 1240
Set Height 720
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set WindowBar Colorful

Hide
Type "mkdir -p .tmp/demo && source /tmp/agentctl-demo/env.sh && clear" Enter
Sleep 800ms
Show

Type "agentctl cliproxy install --port 18318" Sleep 400ms Enter
Wait+Screen@3m /agentctl cliproxy login/
Sleep 2.5s
Type "agentctl cliproxy keys" Sleep 400ms Enter
Sleep 3s

Type "agentctl" Sleep 400ms Enter
Sleep 3s
# Servers › CLIProxyAPI, then its keys.
Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms
Sleep 2s
Type "1" Sleep 3.5s
Escape Sleep 1s
# Log in: Claude is last and asks first.
Type "2" Sleep 2s
Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms Down Sleep 250ms
Sleep 1s
Enter Sleep 4s
Type "n" Sleep 1s
Escape Sleep 800ms
# Server menu.
Type "3" Sleep 3s
Escape Sleep 800ms
Type "q" Sleep 1s
