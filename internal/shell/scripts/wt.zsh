# wt shell integration for zsh. Load it from ~/.zshrc with:
#   eval "$(git-wt shell init zsh)"
#
# wt runs git-wt with its arguments and moves the shell to the directory the
# binary writes to the file named by WT_DIRECTIVE_CD_FILE. Both variables
# are passed to that run only. __wt_previous_dir is not exported, so a child
# shell starts with no previous directory.
wt() {
  local __wt_tmp __wt_rc __wt_dest __wt_from
  __wt_tmp=$(command mktemp "${TMPDIR:-/tmp}/wt.XXXXXX") || return
  {
    WT_DIRECTIVE_CD_FILE=$__wt_tmp WT_PREVIOUS_DIR=${__wt_previous_dir-} command git-wt "$@"
    __wt_rc=$?
    if [[ -s $__wt_tmp ]]; then
      __wt_dest=$(<"$__wt_tmp")
      __wt_from=$PWD
      if builtin cd -- "$__wt_dest"; then
        typeset -g __wt_previous_dir=$__wt_from
      elif (( __wt_rc == 0 )); then
        __wt_rc=1
      fi
    fi
  } always {
    command rm -f -- "$__wt_tmp"
  }
  return $__wt_rc
}
