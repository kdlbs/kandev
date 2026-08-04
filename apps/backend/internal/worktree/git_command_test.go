package worktree

import (
	"context"
	"reflect"
	"testing"
)

func TestNewGitCommandAddsLongPathConfig(t *testing.T) {
	cmd := newGitCommand(context.Background(), "status", "--short")
	want := []string{"git", "-c", "core.longpaths=true", "status", "--short"}

	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("newGitCommand args = %v, want %v", cmd.Args, want)
	}
}
