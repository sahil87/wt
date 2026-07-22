package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// NavigateTo is the single shell-cd implementation shared by `wt go` and the
// launcher's "Open here" action (launcher-contract.md §3, v2). It records a
// navigation target for the shell wrapper and the scripting form:
//
//  1. Writes the resolved absolute path to WT_CD_FILE when the env var is set
//     (mode 0600, truncate-on-write) so the wt() shell wrapper cd's the parent
//     shell there. The write runs FIRST — it is the one operation that can
//     fail, and emitting the success confirmation before a failed write would
//     mislead. A write failure returns an error with no output emitted.
//  2. When WT_CD_FILE is unset and WT_WRAPPER != "1", prints the two-line
//     "shell wrapper" hint to stderr (the WT_WRAPPER-gated hint convention,
//     launcher-contract.md §4).
//  3. Emits a compact-arrow confirmation to STDERR so the user can see where
//     they are landing: "→ {repoName} / {basename}  ({branch})" plus the
//     two-space-indented absolute path. Outside a git context (repoName == "")
//     the first line degrades to "→ {basename}". Plain text, no color — so it
//     is NO_COLOR-safe by construction.
//  4. ALWAYS prints the bare resolved path to stdout as the last line, so the
//     no-wrapper scripting form works: cd "$(command wt go some-name)" /
//     cd "$(command wt open <path> -a open_here)".
//
// Per Constitution VII, wt never cd's the parent shell directly — it
// cooperates via WT_CD_FILE / stdout and the shell wrapper evaluates the
// result. stdout stays exactly the bare resolved path (the machine contract);
// all human copy is on stderr.
func NavigateTo(path, repoName, branch string) error {
	if cdFile := os.Getenv("WT_CD_FILE"); cdFile != "" {
		if err := os.WriteFile(cdFile, []byte(path), 0600); err != nil {
			return fmt.Errorf("writing navigation target to WT_CD_FILE: %w", err)
		}
	} else if os.Getenv("WT_WRAPPER") != "1" {
		fmt.Fprintln(os.Stderr, `hint: cd needs the shell wrapper. Run: eval "$(wt shell-init zsh)" (or bash)`)
		fmt.Fprintln(os.Stderr, `      Add it to your ~/.zshrc or ~/.bashrc to make it permanent.`)
	}

	// Confirmation block (stderr, human copy). Printed only after the
	// WT_CD_FILE write (above) has succeeded — there are no further
	// error/exit conditions below.
	if repoName != "" {
		fmt.Fprintf(os.Stderr, "→ %s / %s  (%s)\n", repoName, filepath.Base(path), branch)
	} else {
		fmt.Fprintf(os.Stderr, "→ %s\n", filepath.Base(path))
	}
	fmt.Fprintf(os.Stderr, "  %s\n", path)

	// Always emit the resolved path as the last stdout line (the machine
	// contract for the scripting form).
	fmt.Println(path)
	return nil
}
