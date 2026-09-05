package worktree

import (
	"reflect"
	"testing"
)

func TestParseDirtyWorktreeFiles(t *testing.T) {
	status := " M tracked.txt\x00?? untracked.txt\x00R  renamed-new.txt\x00renamed-old.txt\x00C  copied-new.txt\x00copied-old.txt\x00"
	got := parseDirtyWorktreeFiles(status)
	want := []string{"copied-new.txt", "renamed-new.txt", "tracked.txt", "untracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dirty files = %#v, want %#v", got, want)
	}
}

func TestParseDirtyWorktreeFilesPreservesPathWhitespace(t *testing.T) {
	status := "??  leading and trailing.txt \x00"
	got := parseDirtyWorktreeFiles(status)
	want := []string{" leading and trailing.txt "}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dirty files = %#v, want %#v", got, want)
	}
}
