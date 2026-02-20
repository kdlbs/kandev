package agents

import (
	"testing"
)

func TestResumeAt_Empty(t *testing.T) {
	cmd := Cmd("agent").ResumeAt(NewParam("--resume-session-at"), "").Build()
	args := cmd.Args()
	for _, arg := range args {
		if arg == "--resume-session-at" {
			t.Error("ResumeAt should not append flag when uuid is empty")
		}
	}
}

func TestResumeAt_WithUUID(t *testing.T) {
	cmd := Cmd("agent").ResumeAt(NewParam("--resume-session-at"), "msg-uuid-123").Build()
	args := cmd.Args()

	foundFlag := false
	for i, arg := range args {
		if arg == "--resume-session-at" {
			foundFlag = true
			if i+1 >= len(args) {
				t.Fatal("--resume-session-at flag has no value")
			}
			if args[i+1] != "msg-uuid-123" {
				t.Errorf("--resume-session-at value = %q, want %q", args[i+1], "msg-uuid-123")
			}
			break
		}
	}
	if !foundFlag {
		t.Error("--resume-session-at flag not found in args")
	}
}

func TestResumeAt_EmptyFlag(t *testing.T) {
	cmd := Cmd("agent").ResumeAt(Param{}, "msg-uuid-123").Build()
	args := cmd.Args()
	if len(args) != 1 || args[0] != "agent" {
		t.Errorf("ResumeAt with empty flag should not modify args, got %v", args)
	}
}
