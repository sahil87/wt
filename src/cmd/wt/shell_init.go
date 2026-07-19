package main

import (
	"os"

	"github.com/spf13/cobra"

	wt "github.com/sahil87/wt/internal/worktree"
)

// ShellWrapperFunc is the shell wrapper function output by shell-init.
// Defined as a constant to avoid duplication between the subcommand and tests.
// The wrapper is valid source for both supported shells (zsh and bash).
const ShellWrapperFunc = `wt() {
  local _wt_cd _wt_rc
  _wt_cd=$(mktemp "${TMPDIR:-/tmp}/wt-cd.XXXXXX")
  WT_CD_FILE="$_wt_cd" command wt "$@"
  _wt_rc=$?
  if [[ -s "$_wt_cd" ]]; then
    local _wt_dir
    _wt_dir=$(<"$_wt_cd")
    cd -- "$_wt_dir" || true
  fi
  rm -f "$_wt_cd"
  return $_wt_rc
}
export WT_WRAPPER=1
`

// supportedShells is the set of shells shell-init emits a wrapper for.
var supportedShells = map[string]bool{
	"zsh":  true,
	"bash": true,
}

func shellInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell-init <shell>",
		Short: "Output shell wrapper function for eval",
		Long: `Output a shell wrapper function suitable for eval in your shell profile.

Usage:
  eval "$(wt shell-init zsh)"     # in ~/.zshrc
  eval "$(wt shell-init bash)"    # in ~/.bashrc

Add the matching line to your shell profile to enable the "Open here"
menu option, which changes the current shell's working directory to the
selected worktree.

The shell argument is required (zsh or bash). Everything on stdout is
eval-safe shell source; a missing or unsupported shell is a usage error
(exit 2, message on stderr, nothing on stdout).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Usage errors exit 2 (wt.ExitInvalidArgs) via the direct-exit
			// pattern (as update.go does for ErrBrewNotFound): main.go maps
			// RunE errors to ExitGeneralError (1), so returning an error here
			// would emit the wrong code for a usage error. stdout MUST stay
			// empty on these paths — it is eval'ed verbatim by shell profiles
			// and by the `shll shell-init` composer.
			if len(args) == 0 {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"missing shell argument",
					"wt shell-init requires the target shell so its output is valid source for that shell",
					`run "wt shell-init zsh" or "wt shell-init bash"`)
			}
			shell := args[0]
			if !supportedShells[shell] {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"unsupported shell \""+shell+"\"",
					"wt shell-init supports zsh and bash only",
					`run "wt shell-init zsh" or "wt shell-init bash"`)
			}

			os.Stdout.WriteString(ShellWrapperFunc)
			return nil
		},
	}

	return cmd
}
