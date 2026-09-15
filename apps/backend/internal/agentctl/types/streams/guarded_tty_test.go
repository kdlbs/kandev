package streams

import (
	"context"
	"strings"
	"testing"
)

func TestValidateGuardedTTYArgvUsesBridgeBounds(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{name: "valid", argv: []string{"sh", "-lc", "test -t 0"}, want: true},
		{name: "maximum argument count", argv: maxGuardedTTYArgv(), want: true},
		{name: "maximum element and total bytes", argv: []string{
			strings.Repeat("a", GuardedTTYMaxSingleArgBytes),
			strings.Repeat("b", GuardedTTYMaxSingleArgBytes),
		}, want: true},
		{name: "empty", argv: nil},
		{name: "too many", argv: make([]string, GuardedTTYMaxArgCount+1)},
		{name: "single too large", argv: []string{string(make([]byte, GuardedTTYMaxSingleArgBytes+1))}},
		{name: "total too large", argv: []string{string(make([]byte, GuardedTTYMaxSingleArgBytes)), string(make([]byte, GuardedTTYMaxSingleArgBytes)), "x"}},
		{name: "nul", argv: []string{"bad\x00arg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateGuardedTTYArgv(tt.argv); got != tt.want {
				t.Fatalf("ValidateGuardedTTYArgv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func maxGuardedTTYArgv() []string {
	argv := make([]string, GuardedTTYMaxArgCount)
	for index := range argv {
		argv[index] = "x"
	}
	return argv
}

func TestMCPExecutionContextRequiresCompleteTrustedIdentity(t *testing.T) {
	for _, execution := range []MCPExecutionContext{
		{TaskID: "task-1", SessionID: "session-1"},
		{ExecutionID: "execution-1", SessionID: "session-1"},
		{ExecutionID: "execution-1", TaskID: "task-1"},
	} {
		if _, ok := MCPExecutionContextFromContext(WithMCPExecutionContext(context.Background(), execution)); ok {
			t.Fatalf("incomplete execution context accepted: %+v", execution)
		}
	}
	complete := MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"}
	if got, ok := MCPExecutionContextFromContext(WithMCPExecutionContext(context.Background(), complete)); !ok || got != complete {
		t.Fatalf("complete execution context = %+v, %v", got, ok)
	}
}
