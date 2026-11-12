# Records .tmp/demo/t3.gif: T3 Code's service, its devices and a pairing link. Run `mise run demo`, which sets up
# the scratch world this expects in /tmp/agentctl-demo.
Output .tmp/demo/t3.gif

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

Type "agentctl t3" Sleep 400ms Enter
Sleep 3s
Type "agentctl" Sleep 400ms Enter
Sleep 3s
Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms Down Sleep 300ms
Sleep 2s
# Devices, then a pairing link for a phone over the tailnet.
Enter Sleep 3s
Type "p" Sleep 1.5s
Type "phone" Sleep 500ms Tab Sleep 300ms
Ctrl+U Type "https://studio.tail035df.ts.net" Sleep 600ms Enter
Sleep 5s
Enter Sleep 2.5s
Escape Sleep 800ms
Type "q" Sleep 1s
