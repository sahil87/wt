package worktree

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestParseRecordedBases(t *testing.T) {
	out := "branch.feat/v1.2.wt-base\norigin/main\x00" +
		"branch.feat/v1.2.wt-base-sha\naaa111\x00" +
		"branch.a.b.wt-base\nmain\x00" +
		"branch.a.b.wt-base-sha\nbbb222\x00" +
		"branch.x.remote\norigin\x00" +
		"branch.only-ref.wt-base\nmain\x00"

	got := parseRecordedBases(out)
	want := map[string]RecordedBase{
		"feat/v1.2": {Ref: "origin/main", SHA: "aaa111"},
		"a.b":       {Ref: "main", SHA: "bbb222"},
		"only-ref":  {Ref: "main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseRecordedBases = %v, want %v", got, want)
	}
}

func TestParseRecordedBases_Empty(t *testing.T) {
	if got := parseRecordedBases(""); got == nil || len(got) != 0 {
		t.Errorf("parseRecordedBases(\"\") = %v, want empty non-nil map", got)
	}
}

func TestRecordBaseAndRecordedBases_RoundTrip(t *testing.T) {
	dir, _ := newRollbackFixture(t)
	if got := RecordedBases(); len(got) != 0 {
		t.Fatalf("RecordedBases on a fresh repo = %v, want empty", got)
	}
	if out, err := exec.Command("git", "-C", dir, "branch", "feat/v1.2").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}
	if err := RecordBase("feat/v1.2", "origin/main", "abc123"); err != nil {
		t.Fatal(err)
	}
	got := RecordedBases()
	want := map[string]RecordedBase{"feat/v1.2": {Ref: "origin/main", SHA: "abc123"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RecordedBases = %v, want %v", got, want)
	}
}
