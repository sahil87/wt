package worktree

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatCreateSuccessJSON_AllKeysPresent(t *testing.T) {
	s, err := FormatCreateSuccessJSON(CreateReceipt{
		Path: "/p/w", Name: "w", Branch: StrPtr("b"),
		BaseRef: StrPtr("main"), BaseSHA: StrPtr("abc"), Mode: ModeNew, Init: InitOK,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "\n") {
		t.Errorf("document must be single-line, got %q", s)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["ok"] != true {
		t.Errorf("ok = %v, want true", doc["ok"])
	}
	res := doc["result"].(map[string]any)
	for _, k := range []string{"path", "name", "branch", "base_ref", "base_sha", "mode", "init"} {
		if _, ok := res[k]; !ok {
			t.Errorf("result missing key %q", k)
		}
	}
	if res["mode"] != "new" || res["init"] != "ok" {
		t.Errorf("mode/init = %v/%v", res["mode"], res["init"])
	}
}

func TestFormatCreateSuccessJSON_Nulls(t *testing.T) {
	s, _ := FormatCreateSuccessJSON(CreateReceipt{Path: "/p", Name: "w", Mode: ModeCheckout, Init: InitSkipped})
	var doc struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"branch", "base_ref", "base_sha"} {
		v, ok := doc.Result[k]
		if !ok || v != nil {
			t.Errorf("result[%q] = %v (present=%v), want explicit null", k, v, ok)
		}
	}
}

func TestFormatCreateFailureJSON(t *testing.T) {
	s, err := FormatCreateFailureJSON(2, "Invalid --base ref: x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"ok":false,"error":{"code":2,"message":"Invalid --base ref: x"}}`; s != want {
		t.Errorf("got %s, want %s", s, want)
	}

	s, _ = FormatCreateFailureJSON(7, "init failed", &CreateReceipt{Path: "/p", Name: "w", Mode: ModeNew, Init: InitFailed})
	var doc struct {
		OK     bool `json:"ok"`
		Result struct {
			Init string `json:"init"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OK || doc.Result.Init != "failed" {
		t.Errorf("unexpected doc: %s", s)
	}
}
