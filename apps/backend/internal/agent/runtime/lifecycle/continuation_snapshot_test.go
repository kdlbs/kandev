package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBuildContinuationSnapshotMarksEveryShortenedSection(t *testing.T) {
	const objectivePrefix = "Task objective:\n"
	const messagePrefix = "[user]\n"

	tests := []struct {
		name  string
		input ContinuationSnapshotInput
	}{
		{
			name: "reserved context",
			input: ContinuationSnapshotInput{
				TaskObjective: strings.Repeat("o", continuationReservedBudget),
			},
		},
		{
			name: "final snapshot budget",
			input: ContinuationSnapshotInput{
				TaskObjective: strings.Repeat("o", continuationReservedBudget-len(objectivePrefix)),
				Messages: []*models.Message{{
					AuthorType: "user",
					Content:    strings.Repeat("m", continuationConversationBudget-len(messagePrefix)),
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := BuildContinuationSnapshot(tt.input)
			if !snapshot.Truncated {
				t.Fatalf("snapshot = %+v, want truncation metadata", snapshot)
			}
		})
	}
}

func TestBuildEffectivePromptLogsOnlyBoundedDiagnostics(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	history, err := NewSessionHistoryManager(t.TempDir(), "", log)
	if err != nil {
		t.Fatalf("NewSessionHistoryManager: %v", err)
	}
	const (
		sessionID = "session-continuation-log"
		secret    = "continuation-secret-that-must-not-appear-in-logs"
		prompt    = "follow-up prompt"
	)
	if err := history.AppendUserMessage(sessionID, secret); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	sm := NewSessionManager(log, make(chan struct{}))
	sm.historyManager = history
	execution := &AgentExecution{
		ID:                 "execution-continuation-log",
		SessionID:          sessionID,
		needsResumeContext: true,
	}
	effective := sm.buildEffectivePrompt(execution, prompt)
	if !strings.Contains(effective, secret) {
		t.Fatalf("effective prompt = %q, want the history content", effective)
	}

	entries := logs.FilterMessage("injecting resume context into follow-up prompt").All()
	if len(entries) != 1 {
		t.Fatalf("diagnostic log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if _, ok := fields["resume_prompt"]; ok {
		t.Fatal("diagnostic log retained the full resume prompt")
	}
	for _, field := range []string{"original_length", "effective_length", "original_hash", "effective_hash"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("diagnostic log missing bounded field %q: %v", field, fields)
		}
	}
	originalHash := sha256.Sum256([]byte(prompt))
	if got, want := fields["original_hash"], hex.EncodeToString(originalHash[:]); got != want {
		t.Fatalf("original_hash = %v, want %q", got, want)
	}
	if strings.Contains(fmt.Sprint(logs.All()), secret) || strings.Contains(fmt.Sprint(logs.All()), effective) {
		t.Fatalf("logs contain continuation prompt content: %v", logs.All())
	}
}
