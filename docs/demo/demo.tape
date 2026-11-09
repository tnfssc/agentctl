# Records .tmp/demo/demo.gif. Run `mise run demo`, which sets up the scratch
# world this expects in /tmp/agentctl-demo.
Output .tmp/demo/demo.gif

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

Type "agentctl proxy test home" Sleep 400ms Enter
Sleep 3s

Type "agentctl" Sleep 400ms Enter
Sleep 3s

# Codex → home, keeping its model; review, apply.
Down Sleep 700ms
Enter Sleep 1.2s
Down Sleep 700ms Enter
Sleep 2s
Enter Sleep 3s
Type "y" Sleep 2.5s

# OpenCode → home on Gemini.
Down Sleep 700ms
Enter Sleep 1.2s
Down Sleep 700ms Enter
Sleep 1.5s
Down Sleep 700ms Enter
Sleep 3s
Type "y" Sleep 2.5s

Type "q" Sleep 600ms
Type "agentctl status" Sleep 400ms Enter
Sleep 3.5s
