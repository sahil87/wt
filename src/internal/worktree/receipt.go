package worktree

import "encoding/json"

// CreateMode is the `mode` field of the `wt create --json` receipt.
type CreateMode string

const (
	ModeNew      CreateMode = "new"
	ModeCheckout CreateMode = "checkout"
	ModeReuse    CreateMode = "reuse"
	ModeTemp     CreateMode = "temp"
)

// InitOutcome is the `init` field of the `wt create --json` receipt.
type InitOutcome string

const (
	InitOK     InitOutcome = "ok"
	InitFailed InitOutcome = "failed"
	// InitSkipped: init disabled, or the built-in default was not applicable.
	InitSkipped InitOutcome = "skipped"
	// InitNotFoundSkip: the structured init not-found skip fired. (InitNotFound
	// is already the not-found error type.)
	InitNotFoundSkip InitOutcome = "not-found"
)

// CreateReceipt is the `result` object of the `wt create --json` document. The
// pointer fields have no omitempty so a nil marshals as an explicit null.
type CreateReceipt struct {
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	Branch  *string     `json:"branch"`
	BaseRef *string     `json:"base_ref"`
	BaseSHA *string     `json:"base_sha"`
	Mode    CreateMode  `json:"mode"`
	Init    InitOutcome `json:"init"`
}

type createSuccessDoc struct {
	OK     bool          `json:"ok"`
	Result CreateReceipt `json:"result"`
}

type createErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type createFailureDoc struct {
	OK     bool            `json:"ok"`
	Error  createErrorBody `json:"error"`
	Result *CreateReceipt  `json:"result,omitempty"`
}

// FormatCreateSuccessJSON renders the success document as compact single-line
// JSON (no trailing newline; callers print it with Println).
func FormatCreateSuccessJSON(r CreateReceipt) (string, error) {
	b, err := json.Marshal(createSuccessDoc{OK: true, Result: r})
	return string(b), err
}

// FormatCreateFailureJSON renders the failure document: code is the process
// exit code, message the `Error:` line. r, when non-nil, is the receipt of a
// worktree that was kept (init failure).
func FormatCreateFailureJSON(code int, message string, r *CreateReceipt) (string, error) {
	b, err := json.Marshal(createFailureDoc{
		Error:  createErrorBody{Code: code, Message: message},
		Result: r,
	})
	return string(b), err
}

// StrPtr returns a pointer to s, for populating nullable receipt fields.
func StrPtr(s string) *string { return &s }
