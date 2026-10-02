# wt shell integration for fish 3.3 or later. Load it from
# ~/.config/fish/config.fish with:
#   git-wt shell init fish | source
#
# wt runs git-wt with its arguments and moves the shell to the directory the
# binary writes to the file named by WT_DIRECTIVE_CD_FILE. Both variables
# are passed to that run only. __wt_previous_dir is a global, not exported,
# variable, so a child shell starts with no previous directory.
function wt --description 'Manage git worktrees from the terminal'
    set -l tmpdir /tmp
    if test -n "$TMPDIR"
        set tmpdir $TMPDIR
    end
    set -l tmp (command mktemp "$tmpdir/wt.XXXXXX")
    or return 1
    WT_DIRECTIVE_CD_FILE=$tmp WT_PREVIOUS_DIR="$__wt_previous_dir" command git-wt $argv
    set -l rc $status
    if test -s "$tmp"
        set -l dest (string collect <$tmp)
        set -l from $PWD
        # fish's cd, not builtin cd: it keeps dirprev for prevd and cd -.
        if cd $dest
            set -g __wt_previous_dir $from
        else if test $rc -eq 0
            set rc 1
        end
    end
    command rm -f -- $tmp
    return $rc
end
