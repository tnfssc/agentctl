# Records .tmp/demo/phone.gif: the compact dashboard on a phone-sized terminal over SSH. Run `mise run demo`, which sets up
# the scratch world this expects in /tmp/agentctl-demo.
Output .tmp/demo/phone.gif

Set Shell "bash"
Set FontSize 15
Set Width 540
Set Height 760
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set WindowBar Colorful

Hide
Type "mkdir -p .tmp/demo && source /tmp/agentctl-demo/env.sh && clear" Enter
Sleep 800ms
Show

Type "agentctl" Sleep 400ms Enter
Sleep 3s
Down Sleep 500ms Down Sleep 500ms Down Sleep 500ms Down Sleep 500ms
Sleep 1s
Enter Sleep 2.5s
Escape Sleep 1s
Down Sleep 500ms Down Sleep 500ms Down Sleep 500ms
Sleep 1s
Enter Sleep 3s
Escape Sleep 1s
Type "q" Sleep 1s
