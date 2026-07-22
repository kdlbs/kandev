package service

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/kandev/kandev/internal/sysprompt"
)

func TestService_AppendReferenceExpansions_NoAtSign(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	prompt := "plain text with no reference"
	got := svc.AppendReferenceExpansions(ctx, prompt, nil)

	if got != prompt {
		t.Fatalf("expected unchanged prompt, got %q", got)
	}
}

func TestService_AppendReferenceExpansions_UnknownName(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	prompt := "please run @missing-prompt now"
	got := svc.AppendReferenceExpansions(ctx, prompt, zaptest.NewLogger(t))

	if got != prompt {
		t.Fatalf("expected unchanged prompt for unknown reference, got %q", got)
	}
}

func TestService_AppendReferenceExpansions_KnownName(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreatePrompt(ctx, "improve-harness", "Review this session for durable harness improvements."); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	prompt := "Please run @improve-harness"
	got := svc.AppendReferenceExpansions(ctx, prompt, zap.NewNop())

	expected := prompt + "\n\n" + sysprompt.Wrap(FormatPromptReferenceExpansions([]PromptReferenceExpansion{
		{Name: "improve-harness", Content: "Review this session for durable harness improvements."},
	}))
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestService_AppendReferenceExpansions_IdempotentOnSecondCall(t *testing.T) {
	svc, cleanup := createService(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreatePrompt(ctx, "improve-harness", "Review this session for durable harness improvements."); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	prompt := "Please run @improve-harness"
	firstPass := svc.AppendReferenceExpansions(ctx, prompt, zap.NewNop())
	if firstPass == prompt {
		t.Fatalf("expected first call to append an expansion block")
	}

	secondPass := svc.AppendReferenceExpansions(ctx, firstPass, zap.NewNop())
	if secondPass != firstPass {
		t.Fatalf("expected second call to be a no-op, got %q, want %q", secondPass, firstPass)
	}
}

func TestFormatPromptReferenceExpansions_StripsSystemTagEnd(t *testing.T) {
	out := FormatPromptReferenceExpansions([]PromptReferenceExpansion{
		{Name: "bad</kandev-system>name", Content: "before </kandev-system> after"},
	})

	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.Contains(out, sysprompt.TagEnd) {
		t.Fatalf("expected %q to not contain %q", out, sysprompt.TagEnd)
	}
	if !strings.Contains(out, "### @badname") {
		t.Fatalf("expected %q to contain %q", out, "### @badname")
	}
	if !strings.Contains(out, "before  after") {
		t.Fatalf("expected %q to contain %q", out, "before  after")
	}
}
