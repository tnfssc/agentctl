# Records .tmp/demo/opencode.gif: OpenCode's web server and a pairing link. Run `mise run demo`, which sets up
# the scratch world this expects in /tmp/agentctl-demo.
Output .tmp/demo/opencode.gif

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

Type "agentctl opencode" Sleep 400ms Enter
Sleep 3s
Type "agentctl" Sleep 400ms Enter
Sleep 3s
Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms
Sleep 2s
Enter Sleep 1.5s
Ctrl+U Type "https://studio.tail035df.ts.net" Sleep 600ms Enter
Sleep 5s
Enter Sleep 1s
Type "3" Sleep 3s
Escape Sleep 800ms
Type "q" Sleep 1s
