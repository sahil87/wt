package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Kind enumerates the reasons an init-script invocation could not be resolved
// to a runnable command. It is a named type so the Go compiler can flag
// unhandled cases in switches when new kinds are added.
type Kind int

const (
	// CommandNotOnPath: the init-script value parsed as a command invocation
	// (whitespace-separated tokens) and the first token was not found via
	// exec.LookPath.
	CommandNotOnPath Kind = iota
	// FileNotFound: the init-script value parsed as a path (no whitespace) and
	// the file did not exist under repoRoot.
	FileNotFound
)

// Hint suffixes for not-found warnings. File-scoped constants so the two
// branches share the canonical phrasing without duplicating string literals
// at call sites.
const (
	hintCommandNotOnPath = "Install fab-kit or set WORKTREE_INIT_SCRIPT to a custom script."
	hintFileNotFound     = "Create the file or set WORKTREE_INIT_SCRIPT to a custom script."
)

// InitNotFound is the structured "not found" outcome returned by
// ResolveInitInvocation. Callers render a verbose warning via RenderWarning
// and continue without running an init step (the not-found case is non-fatal
// per the init-protocol contract).
type InitNotFound struct {
	Kind Kind
	// Name is the first whitespace-separated token of the init-script string.
	// Populated when Kind == CommandNotOnPath.
	Name string
	// Path is the resolved absolute filesystem path that was checked.
	// Populated when Kind == FileNotFound.
	Path string
	// RelPath is the init-script string as the user supplied it (so warnings
	// can echo it back literally). Populated when Kind == FileNotFound.
	RelPath string
}

// RenderWarning returns the verbose user-facing warning for this not-found
// outcome. Both wt init and wt create's init step call this — keeping the
// rendering on the type prevents the two call sites from drifting.
func (n InitNotFound) RenderWarning() string {
	switch n.Kind {
	case CommandNotOnPath:
		return fmt.Sprintf("Warning: %q not found on PATH, skipping init\n%s",
			n.Name, hintCommandNotOnPath)
	case FileNotFound:
		return fmt.Sprintf("No init script found at: %s\n\nTo add an init script:\n  mkdir -p %s\n  touch %s\n%s",
			n.Path, filepath.Dir(n.RelPath), n.RelPath, hintFileNotFound)
	}
	return ""
}

// ResolveInitInvocation parses initScript and either returns a runnable
// *exec.Cmd or a structured *InitNotFound describing why resolution failed.
//
// Resolution rules mirror docs/specs/init-protocol.md:
//   - If initScript contains whitespace, treat the first token as a command
//     name and pass the rest as arguments. Use exec.LookPath to verify the
//     command is on PATH.
//   - Otherwise, treat initScript as a path relative to repoRoot and verify
//     the file exists.
//
// Return-value contract:
//   - On success: (*exec.Cmd, nil, nil). Dir/Stdout/Stderr/Stdin are left
//     unset; callers wire those (wt init and wt create use different working
//     directories).
//   - On structured not-found: (nil, *InitNotFound, nil). Not-found is a
//     successful resolution outcome, not an error.
//   - On unexpected failure: (nil, nil, error). Reserved for cases callers
//     cannot recover from (e.g., an empty/un-tokenizable init-script string).
//
// On Unix, the returned cmd has SysProcAttr.Setpgid = true so cmd.Run()
// runs the child in its own process group. This lets the SIGINT-during-init
// handler in wt create signal the whole group (script + any children) by
// targeting -cmd.Process.Pid.
func ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error) {
	trimmed := strings.TrimSpace(initScript)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("init-script string is empty")
	}

	var cmd *exec.Cmd
	if strings.ContainsAny(trimmed, " \t") {
		parts := strings.Fields(trimmed)
		if _, err := exec.LookPath(parts[0]); err != nil {
			return nil, &InitNotFound{Kind: CommandNotOnPath, Name: parts[0]}, nil
		}
		cmd = exec.Command(parts[0], parts[1:]...)
	} else {
		// File path: resolve relative to repoRoot.
		scriptPath := filepath.Join(repoRoot, trimmed)
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return nil, &InitNotFound{Kind: FileNotFound, Path: scriptPath, RelPath: trimmed}, nil
		} else if err != nil {
			return nil, nil, fmt.Errorf("stat init script %s: %w", scriptPath, err)
		}
		cmd = exec.Command("bash", scriptPath)
	}

	setInitProcessGroup(cmd)
	return cmd, nil, nil
}
