# Records .tmp/demo/tune.gif: curated Claude Code settings: tick, review the diff, apply. Run `mise run demo`, which sets up
# the scratch world this expects in /tmp/agentctl-demo.
Output .tmp/demo/tune.gif

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

Type "agentctl" Sleep 400ms Enter
Sleep 3s
# Settings › Claude Code.
Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms
Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms Down Sleep 200ms
Sleep 2s
Enter Sleep 3s
Down Sleep 700ms Down Sleep 700ms Down Sleep 700ms
Space Sleep 1s
# Review: the risky item asks first, then the diff, then apply.
Type "y" Sleep 3s
Type "y" Sleep 4s
Type "y" Sleep 3s
Type "q" Sleep 600ms
Type "head -16 ~/.claude/settings.json" Sleep 400ms Enter
Sleep 4s
