# Records docs/assets/demo.gif with VHS (https://github.com/charmbracelet/vhs).
#
#   brew install vhs
#   go build -o /tmp/wt-demo/bin/git-wt ./cmd/git-wt
#   vhs docs/assets/demo.tape
#
# The hidden part builds a throwaway workspace with two repositories under
# /tmp/wt-demo/code, so the recording never shows your own machine.

Output docs/assets/demo.gif

Set Shell zsh
Set FontSize 18
Set Width 1200
Set Height 640
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms

Hide
Type 'export PATH=/tmp/wt-demo/bin:$PATH WT_REPOS_ROOT=/tmp/wt-demo/code PS1="%F{blue}%1~%f ❯ "' Enter
Type 'rm -rf /tmp/wt-demo/code && mkdir -p /tmp/wt-demo/code' Enter
Type 'for r in api web; do git init -q -b main /tmp/wt-demo/code/$r && git -C /tmp/wt-demo/code/$r commit -q --allow-empty -m init; done' Enter
Type 'eval "$(git-wt shell init zsh)" && cd /tmp/wt-demo/code/api && clear' Enter
Show

Type "wt create auth-fix" Sleep 400ms Enter
Sleep 1.5s
Type "wt cd web" Sleep 400ms Enter
Sleep 1s
Type "wt create dark-mode" Sleep 400ms Enter
Sleep 1.5s
Type "wt list --all-repos" Sleep 400ms Enter
Sleep 3s
Type "wt each --all-repos git branch --show-current" Sleep 400ms Enter
Sleep 3s
Type "wt cd api:auth-fix" Sleep 400ms Enter
Sleep 1s
Type "wt remove @" Sleep 400ms Enter
Sleep 2.5s
