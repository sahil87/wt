# Code Review

<!-- Optional review policy consumed by the validation sub-agent during review.
     Projects opt in by creating this file. All sections are independently optional.
     Delete or leave empty any section that doesn't apply to your project.

     This file guides the REVIEWING agent (critic). For the WRITING agent (author),
     see code-quality.md. Different cognitive modes, different concerns. -->

## Severity Definitions

<!-- How findings are prioritized. The review sub-agent classifies each finding
     into one of these tiers. Override the defaults below to match your project's
     quality bar. -->

- **Must-fix**: Spec mismatches, failing tests, checklist violations — always addressed during rework
- **Should-fix**: Code quality issues, pattern inconsistencies — addressed when clear and low-effort
- **Nice-to-have**: Style suggestions, minor improvements — may be skipped

## Review Scope

<!-- What the review sub-agent inspects. Adjust to exclude generated code,
     vendor directories, or other paths that shouldn't be reviewed. -->

- Changed files only (files touched during apply)
- Skip generated code and vendor directories
- Skip binary files and assets

## False Positive Policy

<!-- How to suppress or override findings the reviewer flags incorrectly.
     Use inline comments in source code to mark intentional deviations. -->

- Inline `<!-- review-ignore: {reason} -->` in markdown files
- Inline `// review-ignore: {reason}` or `# review-ignore: {reason}` in code files
- Suppressed findings are noted in the review report but not counted as failures

## Rework Budget

<!-- Max auto-rework cycles before escalating to the user.
     Applies to /fab-fff and /fab-ff auto-rework loops. -->

- Max cycles: 3
- After 2 consecutive "fix code" attempts on the same issue, escalate to "revise tasks" or "revise spec"

## Project-Specific Review Rules

<!-- Add project-specific review rules here. Examples:
     - All public APIs need integration tests
     - No new dependencies without justification in the spec
     - Database migrations must be reversible
     - All user-facing strings must be internationalized -->

- Tests that invoke the `wt` binary MUST NOT leak side-effects to the host system. Specifically, tests exercising `--worktree-open` or `--app` codepaths SHALL satisfy one of: (a) use a non-side-effecting target (`--worktree-open=skip`, `--app=open_here`, `--app=copy_*`, or any value that fails resolution before shelling out); (b) rely on `runWt`'s default env isolation, which clears `TMUX`, `BYOBU_BACKEND`, `BYOBU_TTY`, `BYOBU_SESSION`, `BYOBU_CONFIG_DIR`, and `TERM_PROGRAM`; or (c) explicitly register `t.Cleanup` to reap any tmux windows / sessions / byobu tabs the test creates. Patterns (a) and (b) are strongly preferred — actual window creation should be exercised only by hand or via dedicated end-to-end fixtures, not in the standard unit-test suite.
