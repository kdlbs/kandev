package acp

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	antigravityOutcomeSessionID = "antigravity-outcome-session"
	antigravityOldQuotaPayload  = "Error 429, Message: old quota, Status: RESOURCE_EXHAUSTED\nreason:QUOTA_EXHAUSTED\n"
	antigravityNewQuotaPayload  = "Error 429, Message: You have reached your quota., Status: RESOURCE_EXHAUSTED\nreason:QUOTA_EXHAUSTED\nquotaResetDelay:2h0m0s\n"
)

type antigravityOutcomeAgent struct {
	burstAgent
	conn       *acpsdk.AgentSideConnection
	sessionID  string
	version    string
	onPrompt   func()
	promptErr  error
	stopReason acpsdk.StopReason
	mu         sync.Mutex
	prompts    int
}

func (a *antigravityOutcomeAgent) Initialize(
	_ context.Context,
	req acpsdk.InitializeRequest,
) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: req.ProtocolVersion,
		AgentInfo:       &acpsdk.Implementation{Name: "antigravity", Version: a.version},
	}, nil
}

func (a *antigravityOutcomeAgent) NewSession(
	context.Context,
	acpsdk.NewSessionRequest,
) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(a.sessionID)}, nil
}

func (a *antigravityOutcomeAgent) Prompt(
	context.Context,
	acpsdk.PromptRequest,
) (acpsdk.PromptResponse, error) {
	a.mu.Lock()
	a.prompts++
	onPrompt, promptErr, stopReason := a.onPrompt, a.promptErr, a.stopReason
	a.mu.Unlock()
	if onPrompt != nil {
		onPrompt()
	}
	if promptErr != nil {
		return acpsdk.PromptResponse{}, promptErr
	}
	if stopReason == "" {
		stopReason = acpsdk.StopReasonEndTurn
	}
	return acpsdk.PromptResponse{StopReason: stopReason}, nil
}

func (a *antigravityOutcomeAgent) promptCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prompts
}

func newAntigravityOutcomeFixture(
	t *testing.T,
	fake *antigravityOutcomeAgent,
	journalRoot string,
) *Adapter {
	t.Helper()
	a := newTestAdapterForAgent(acpcompat.AntigravityAgentID)
	a.antigravityJournalRoot = journalRoot
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake.conn = acpsdk.NewAgentSideConnection(fake, a2cW, c2aR)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	drainEvents(a)
	return a
}

func newAntigravityJournalRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), acpcompat.AntigravityConfigRelDir)
	if err := os.MkdirAll(filepath.Join(root, acpcompat.AntigravityConversationsDir), 0o700); err != nil {
		t.Fatalf("create journal root: %v", err)
	}
	return root
}

func createAntigravityJournal(t *testing.T, root, sessionID, schema string) string {
	t.Helper()
	path := filepath.Join(root, acpcompat.AntigravityConversationsDir, sessionID+".db")
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open journal fixture: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("create journal schema: %v", err)
	}
	return path
}

const antigravityStepsSchema = `CREATE TABLE steps (idx INTEGER PRIMARY KEY, step_type INTEGER NOT NULL, step_payload BLOB)`

func tryCreateAntigravityJournal(root, sessionID, schema string) error {
	path := filepath.Join(root, acpcompat.AntigravityConversationsDir, sessionID+".db")
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	_, err = database.Exec(schema)
	return err
}

func tryInsertAntigravityStep(path string, idx int64, stepType int, payload []byte) error {
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	_, err = database.Exec(
		`INSERT INTO steps (idx, step_type, step_payload) VALUES (?, ?, ?)`,
		idx, stepType, payload,
	)
	return err
}

func insertAntigravityStep(t *testing.T, path string, idx int64, stepType int, payload []byte) {
	t.Helper()
	if err := tryInsertAntigravityStep(path, idx, stepType, payload); err != nil {
		t.Fatalf("insert journal step: %v", err)
	}
}

func antigravityErrorEvent(t *testing.T, a *Adapter) AgentEvent {
	t.Helper()
	event := waitForEventType(t, a, streams.EventTypeError)
	if event.ProviderError == nil || !event.ProviderError.Valid() {
		t.Fatalf("provider error = %+v, want a valid bounded diagnostic", event.ProviderError)
	}
	if event.Error != event.ProviderError.Message {
		t.Fatalf("event error = %q, provider message = %q", event.Error, event.ProviderError.Message)
	}
	if event.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("disposition = %q, want retain_runtime", event.PromptFailureDisposition)
	}
	return event
}

func assertNoAntigravityComplete(t *testing.T, a *Adapter) {
	t.Helper()
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete {
			t.Fatalf("unexpected complete event after a failed turn: %+v", event)
		}
	}
}

func TestAntigravityCurrentTurnOutcome(t *testing.T) {
	t.Run("new error step fails the turn", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		insertAntigravityStep(t, path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityOldQuotaPayload))
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := tryInsertAntigravityStep(path, 2, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload)); err != nil {
					t.Errorf("insert new error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if event.ProviderError.Source != streams.ProviderErrorSourceAntigravityNative {
			t.Fatalf("provider source = %q, want native", event.ProviderError.Source)
		}
		if !strings.Contains(event.Error, "Antigravity quota reached") {
			t.Fatalf("error = %q, want the normalized quota cause", event.Error)
		}
		if event.ProviderError.ResetAt == nil {
			t.Fatal("reset hint = nil, want the provider's relative delay")
		}
		assertNoAntigravityComplete(t, a)
		if a.GetSessionID() != antigravityOutcomeSessionID {
			t.Fatalf("session changed to %q", a.GetSessionID())
		}
	})

	t.Run("stale error step is ignored", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		insertAntigravityStep(t, path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityOldQuotaPayload))
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 1 {
			t.Fatalf("complete generation = %d, want 1", event.PromptGeneration)
		}
	})

	t.Run("no-store boundary inspects a journal created during the turn", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), acpcompat.AntigravityConfigRelDir)
		path := filepath.Join(root, acpcompat.AntigravityConversationsDir, antigravityOutcomeSessionID+".db")
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := os.MkdirAll(filepath.Join(root, acpcompat.AntigravityConversationsDir), 0o700); err != nil {
					t.Errorf("create journal root: %v", err)
					return
				}
				if err := tryCreateAntigravityJournal(root, antigravityOutcomeSessionID, antigravityStepsSchema); err != nil {
					t.Errorf("create new journal: %v", err)
					return
				}
				if err := tryInsertAntigravityStep(path, 0, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload)); err != nil {
					t.Errorf("insert error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if !strings.Contains(event.Error, "Antigravity quota reached") {
			t.Fatalf("error = %q, want quota failure", event.Error)
		}
	})

	t.Run("new database is inspected", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := filepath.Join(root, acpcompat.AntigravityConversationsDir, antigravityOutcomeSessionID+".db")
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := tryCreateAntigravityJournal(root, antigravityOutcomeSessionID, antigravityStepsSchema); err != nil {
					t.Errorf("create new journal: %v", err)
					return
				}
				if err := tryInsertAntigravityStep(path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload)); err != nil {
					t.Errorf("insert new error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if !strings.Contains(event.Error, "Antigravity quota reached") {
			t.Fatalf("error = %q, want a quota failure from the new database", event.Error)
		}
	})

	t.Run("successful follow-up completes", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := tryInsertAntigravityStep(path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload)); err != nil {
					t.Errorf("insert error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "fail once", nil, 1); err != nil {
			t.Fatalf("first Prompt: %v", err)
		}
		antigravityErrorEvent(t, a)
		fake.mu.Lock()
		fake.onPrompt = nil
		fake.mu.Unlock()
		if err := a.Prompt(context.Background(), "follow up", nil, 2); err != nil {
			t.Fatalf("follow-up Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 2 {
			t.Fatalf("follow-up generation = %d, want 2", event.PromptGeneration)
		}
		if fake.promptCount() != 2 {
			t.Fatalf("provider prompts = %d, want 2", fake.promptCount())
		}
	})

	t.Run("cancellation does not fail", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{
			sessionID:  antigravityOutcomeSessionID,
			version:    acpcompat.AntigravityQuotaRefusalVersion,
			stopReason: acpsdk.StopReasonCancelled,
			onPrompt: func() {
				if err := tryInsertAntigravityStep(path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload)); err != nil {
					t.Errorf("insert error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "cancel me", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 1 {
			t.Fatalf("complete generation = %d, want 1", event.PromptGeneration)
		}
	})

	t.Run("qualified refusal is quota", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{
			sessionID:  antigravityOutcomeSessionID,
			version:    acpcompat.AntigravityQuotaRefusalVersion,
			stopReason: acpsdk.StopReasonRefusal,
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "refuse", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if !strings.Contains(event.Error, "Antigravity quota reached") {
			t.Fatalf("error = %q, want a quota failure from typed refusal", event.Error)
		}
	})

	t.Run("unqualified refusal retains ordinary semantics", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{
			sessionID:  antigravityOutcomeSessionID,
			version:    "1.5.0",
			stopReason: acpsdk.StopReasonRefusal,
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "refuse", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 1 {
			t.Fatalf("complete generation = %d, want 1", event.PromptGeneration)
		}
	})

	t.Run("unknown error payload stays unknown", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := tryInsertAntigravityStep(path, 1, acpcompat.AntigravityErrorStepType, []byte("something unexpected happened")); err != nil {
					t.Errorf("insert unknown error step: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if !strings.Contains(event.Error, "Antigravity unknown provider failure") {
			t.Fatalf("error = %q, want an unknown provider failure", event.Error)
		}
		if event.ProviderError.ResetAt != nil {
			t.Fatalf("reset = %v, want unknown", event.ProviderError.ResetAt)
		}
	})

	t.Run("other sessions are not scanned", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		other := createAntigravityJournal(t, root, "other-session", antigravityStepsSchema)
		insertAntigravityStep(t, other, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload))
		createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 1 {
			t.Fatalf("complete generation = %d, want 1", event.PromptGeneration)
		}
	})

	t.Run("missing root does not invent a failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing-antigravity-store")
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if event := waitForEventType(t, a, streams.EventTypeComplete); event.PromptGeneration != 1 {
			t.Fatalf("complete generation = %d, want 1", event.PromptGeneration)
		}
	})
}

func TestAntigravityOutcomeEvidenceUnavailable(t *testing.T) {
	t.Run("symlinked session database", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		other := createAntigravityJournal(t, root, "other-session", antigravityStepsSchema)
		dbPath := filepath.Join(root, acpcompat.AntigravityConversationsDir, antigravityOutcomeSessionID+".db")
		if err := os.Symlink(other, dbPath); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "symlinked session database")
	})

	t.Run("replaced session database", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		dbPath := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		replacementPath := filepath.Join(root, acpcompat.AntigravityConversationsDir, "replacement.db")
		fake := &antigravityOutcomeAgent{
			sessionID: antigravityOutcomeSessionID,
			version:   acpcompat.AntigravityQuotaRefusalVersion,
			onPrompt: func() {
				if err := tryCreateAntigravityJournal(root, "replacement", antigravityStepsSchema); err != nil {
					t.Errorf("create replacement journal: %v", err)
					return
				}
				if err := os.Rename(replacementPath, dbPath); err != nil {
					t.Errorf("replace journal: %v", err)
				}
			},
		}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "replaced session database")
	})

	t.Run("schema mismatch", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		createAntigravityJournal(t, root, antigravityOutcomeSessionID, `CREATE TABLE unrelated (idx INTEGER PRIMARY KEY)`)
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "schema mismatch")
	})

	t.Run("non-numeric step schema", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		path := createAntigravityJournal(
			t, root, antigravityOutcomeSessionID,
			`CREATE TABLE steps (idx TEXT PRIMARY KEY, step_type TEXT NOT NULL, step_payload TEXT)`,
		)
		insertAntigravityStep(t, path, 1, acpcompat.AntigravityErrorStepType, []byte(antigravityNewQuotaPayload))
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "non-numeric step schema")
	})

	t.Run("missing session database", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "missing session database")
	})

	t.Run("busy read", func(t *testing.T) {
		root := newAntigravityJournalRoot(t)
		dbPath := createAntigravityJournal(t, root, antigravityOutcomeSessionID, antigravityStepsSchema)
		writer, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatalf("open writer: %v", err)
		}
		defer func() { _ = writer.Close() }()
		conn, err := writer.Conn(context.Background())
		if err != nil {
			t.Fatalf("writer connection: %v", err)
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.ExecContext(context.Background(), `PRAGMA locking_mode=EXCLUSIVE`); err != nil {
			t.Fatalf("exclusive locking mode: %v", err)
		}
		if _, err := conn.ExecContext(context.Background(), `BEGIN EXCLUSIVE`); err != nil {
			t.Fatalf("begin exclusive: %v", err)
		}
		defer func() { _, _ = conn.ExecContext(context.Background(), `ROLLBACK`) }()

		fake := &antigravityOutcomeAgent{sessionID: antigravityOutcomeSessionID, version: acpcompat.AntigravityQuotaRefusalVersion}
		a := newAntigravityOutcomeFixture(t, fake, root)
		assertAntigravityOutcomeUnverified(t, a, "busy read")
	})
}

func assertAntigravityOutcomeUnverified(t *testing.T, a *Adapter, name string) {
	t.Helper()
	if err := a.Prompt(context.Background(), "do work", nil, 1); err != nil {
		t.Fatalf("%s: Prompt: %v", name, err)
	}
	event := antigravityErrorEvent(t, a)
	if !strings.Contains(event.Error, "Antigravity turn outcome could not be verified") {
		t.Fatalf("%s: error = %q, want an explicit outcome-verification failure", name, event.Error)
	}
	if event.ProviderError.Source != streams.ProviderErrorSourceAntigravityNative {
		t.Fatalf("%s: provider source = %q", name, event.ProviderError.Source)
	}
	assertNoAntigravityComplete(t, a)
}

func TestAntigravityPromptErrorKeepsSession(t *testing.T) {
	t.Run("structured quota prompt error", func(t *testing.T) {
		fake := &promptErrorContinuityAgent{failures: []error{&acpsdk.RequestError{
			Code:    -32000,
			Message: "Internal error",
			Data: map[string]any{
				"status":  "RESOURCE_EXHAUSTED",
				"reason":  "QUOTA_EXHAUSTED",
				"message": "You have reached your quota.",
			},
		}}}
		a := newPromptErrorContinuityFixture(t, acpcompat.AntigravityAgentID, fake)
		a.antigravityJournalRoot = filepath.Join(t.TempDir(), "missing-antigravity-store")
		connection := a.acpConn
		sessionID := a.sessionID
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.Prompt(ctx, "quota prompt", nil, 11); err != nil {
			t.Fatalf("Prompt returned an error instead of a retained failure: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if event.ProviderError.Source != streams.ProviderErrorSourceAntigravityACP {
			t.Fatalf("provider source = %q, want acp", event.ProviderError.Source)
		}
		if !strings.Contains(event.Error, "Antigravity quota reached") {
			t.Fatalf("error = %q, want a normalized quota cause", event.Error)
		}
		if strings.Contains(strings.ToLower(event.Error), "authentication") {
			t.Fatalf("quota error = %q, must not become authentication", event.Error)
		}
		if err := a.Prompt(ctx, "follow up", nil, 12); err != nil {
			t.Fatalf("follow-up Prompt: %v", err)
		}
		if got := waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration; got != 12 {
			t.Fatalf("follow-up generation = %d, want 12", got)
		}
		if a.acpConn != connection || a.sessionID != sessionID {
			t.Fatal("recoverable prompt error replaced the connection or session")
		}
		if fake.newRuns.Load() != 1 {
			t.Fatalf("ACP sessions created = %d, want 1", fake.newRuns.Load())
		}
	})

	t.Run("generic -32000 is not authentication", func(t *testing.T) {
		fake := &promptErrorContinuityAgent{failures: []error{genericPromptError("Internal error")}}
		a := newPromptErrorContinuityFixture(t, acpcompat.AntigravityAgentID, fake)
		a.antigravityJournalRoot = filepath.Join(t.TempDir(), "missing-antigravity-store")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Prompt(ctx, "generic failure", nil, 21); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if strings.Contains(strings.ToLower(event.Error), "authentication") {
			t.Fatalf("generic -32000 = %q, must not be authentication", event.Error)
		}
		if !strings.Contains(event.Error, "Antigravity unknown provider failure") {
			t.Fatalf("generic -32000 = %q, want unknown provider failure", event.Error)
		}
	})

	t.Run("real auth wording is authentication", func(t *testing.T) {
		fake := &promptErrorContinuityAgent{failures: []error{&acpsdk.RequestError{
			Code: -32000, Message: "Authentication required",
		}}}
		a := newPromptErrorContinuityFixture(t, acpcompat.AntigravityAgentID, fake)
		a.antigravityJournalRoot = filepath.Join(t.TempDir(), "missing-antigravity-store")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Prompt(ctx, "auth failure", nil, 31); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		event := antigravityErrorEvent(t, a)
		if !strings.Contains(event.Error, "Antigravity authentication required") {
			t.Fatalf("auth error = %q, want the normalized auth cause", event.Error)
		}
	})
}
