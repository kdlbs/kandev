package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newMCPLoggingManager(t *testing.T, durable bool) (*Manager, *observer.ObservedLogs) {
	t.Helper()
	log, observed := newObservedTestLogger(t)
	cfg := &config.InstanceConfig{
		WorkDir: t.TempDir(), SessionID: "session", Protocol: agent.ProtocolACP,
		AgentArgs: fixtureArgs(), AgentEnv: fixtureEnvSlice("cat"),
	}
	if durable {
		cfg.DurableJournalPath = filepath.Join(t.TempDir(), "delivery.bbolt")
	}
	m := NewManager(cfg, log)
	t.Cleanup(func() {
		require.NoError(t, m.Stop(context.Background()))
		require.NoError(t, m.closeDeliveryJournal())
	})
	return m, observed
}

func publishMCPLoggingEvent(m *Manager, attempt bool) {
	if attempt {
		m.PublishMCPAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "late"})
		return
	}
	m.PublishMCPAttachment(streams.MCPAttachmentEvidence{
		AttemptID: "late", ServerName: "kandev", ConnectionID: "connection",
		Kind: streams.MCPAttachmentEvidenceConnectionClosed,
	})
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19
func TestMCPAttachmentShutdownClosedWriterLogsDebug(t *testing.T) {
	for _, attempt := range []bool{false, true} {
		t.Run(fmt.Sprintf("attempt=%t", attempt), func(t *testing.T) {
			m, observed := newMCPLoggingManager(t, true)
			require.NoError(t, m.Start(context.Background()))
			publishMCPLoggingEvent(m, attempt)
			require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All())
			require.NoError(t, m.Stop(context.Background()))
			store, err := m.DeliveryJournal()
			require.NoError(t, err)
			before, _, err := store.Replay(context.Background(), "session", 0, 100)
			require.NoError(t, err)
			require.NotEmpty(t, before)
			observed.TakeAll()

			publishMCPLoggingEvent(m, attempt)

			entries := observed.All()
			require.Len(t, entries, 1)
			require.Equal(t, zapcore.DebugLevel, entries[0].Level)
			require.Equal(t, errDeliveryWriterClosed.Error(), entries[0].ContextMap()["error"])
			after, _, err := store.Replay(context.Background(), "session", 0, 100)
			require.NoError(t, err)
			require.Equal(t, before, after, "late evidence must not bypass the closed writer")
			select {
			case event := <-m.GetUpdates():
				t.Fatalf("uncommitted event was published: %+v", event)
			default:
			}
		})
	}
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.20
func TestMCPAttachmentCommitErrorClassification(t *testing.T) {
	for _, status := range []Status{StatusRunning, StatusStarting, StatusStopping, StatusStopped} {
		for _, attempt := range []bool{false, true} {
			for _, failure := range []error{
				fmt.Errorf("wrapped: %w", errDeliveryWriterClosed), journal.ErrJournalCorrupt,
				journal.ErrJournalClosed, context.Canceled, context.DeadlineExceeded,
				fmt.Errorf("%s", errDeliveryWriterClosed),
			} {
				t.Run(fmt.Sprintf("%s/attempt=%t/%v", status, attempt, failure), func(t *testing.T) {
					log, observed := newObservedTestLogger(t)
					m := &Manager{
						cfg:    &config.InstanceConfig{DurableJournalPath: "unavailable"},
						logger: log, deliveryJournalErr: failure,
					}
					m.status.Store(status)
					publishMCPLoggingEvent(m, attempt)
					entries := observed.All()
					require.Len(t, entries, 1)
					want := zapcore.ErrorLevel
					if errors.Is(failure, errDeliveryWriterClosed) && (status == StatusStopping || status == StatusStopped) {
						want = zapcore.DebugLevel
					}
					require.Equal(t, want, entries[0].Level)
					require.Equal(t, failure.Error(), entries[0].ContextMap()["error"])
					message := "failed to commit durable MCP attachment event"
					if attempt {
						message = "failed to commit durable MCP attachment attempt"
					}
					require.Equal(t, message, entries[0].Message)
				})
			}
		}
	}
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.21
func TestMCPAttachmentLegacyPublicationUnchanged(t *testing.T) {
	m, observed := newMCPLoggingManager(t, false)
	for _, attempt := range []bool{false, true} {
		publishMCPLoggingEvent(m, attempt)
		select {
		case event := <-m.GetUpdates():
			require.Equal(t, streams.EventTypeMCPAttachment, event.Type)
			require.Zero(t, event.DeliverySequence)
			if attempt {
				require.NotNil(t, event.MCPAttachmentAttempt)
				require.Equal(t, "late", event.MCPAttachmentAttempt.AttemptID)
			} else {
				require.NotNil(t, event.MCPAttachment)
				require.Equal(t, streams.MCPAttachmentEvidenceConnectionClosed, event.MCPAttachment.Kind)
			}
		default:
			t.Fatal("legacy event was not delivered")
		}
	}
	require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All())
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.21
func TestMCPAttachmentPublicationAfterWriterRestart(t *testing.T) {
	m, observed := newMCPLoggingManager(t, true)
	require.NoError(t, m.Start(context.Background()))
	store, err := m.DeliveryJournal()
	require.NoError(t, err)
	publishMCPLoggingEvent(m, false)
	before, _, err := store.Replay(context.Background(), "session", 0, 100)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.NoError(t, m.Stop(context.Background()))
	require.NoError(t, m.Start(context.Background()))
	publishMCPLoggingEvent(m, true)
	after, _, err := store.Replay(context.Background(), "session", 0, 100)
	require.NoError(t, err)
	require.Len(t, after, 2)
	require.Equal(t, before[0], after[0])
	require.Equal(t, before[0].Sequence+1, after[1].Sequence)
	require.Equal(t, streams.EventTypeMCPAttachment, after[1].Type)
	require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All())
}

type mcpEvidenceOnCloseAdapter struct {
	adapter.AgentAdapter
	observe func()
}

func (a *mcpEvidenceOnCloseAdapter) Close() error {
	a.observe()
	return a.AgentAdapter.Close()
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19
func TestMCPAttachmentPermanentTeardownLogsDebug(t *testing.T) {
	for _, attempt := range []bool{false, true} {
		t.Run(fmt.Sprintf("attempt=%t", attempt), func(t *testing.T) {
			m, observed := newMCPLoggingManager(t, true)
			require.NoError(t, m.Start(context.Background()))
			publishMCPLoggingEvent(m, attempt)
			m.adapter = &mcpEvidenceOnCloseAdapter{
				AgentAdapter: m.adapter,
				observe:      func() { publishMCPLoggingEvent(m, attempt) },
			}
			observed.TakeAll()
			require.NoError(t, m.StopForTeardown(context.Background()))
			publishMCPLoggingEvent(m, attempt)
			require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All())
			message := "failed to commit durable MCP attachment event"
			if attempt {
				message = "failed to commit durable MCP attachment attempt"
			}
			require.Len(t, observed.FilterMessage(message).All(), 2)
			_, err := m.DeliveryJournal()
			require.ErrorIs(t, err, journal.ErrJournalClosed)
		})
	}
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.20
func TestMCPAttachmentCanceledShutdownKeepsStorageFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, failure := range []error{
		journal.ErrJournalCorrupt, journal.ErrJournalFull, context.DeadlineExceeded,
		errors.Join(context.Canceled, journal.ErrJournalCorrupt),
		errors.Join(journal.ErrJournalClosed, journal.ErrJournalCorrupt),
	} {
		for _, attempt := range []bool{false, true} {
			t.Run(fmt.Sprintf("attempt=%t/%v", attempt, failure), func(t *testing.T) {
				log, observed := newObservedTestLogger(t)
				m := &Manager{
					cfg:    &config.InstanceConfig{DurableJournalPath: "unavailable"},
					logger: log, deliveryJournalErr: failure, lifetimeCtx: ctx,
				}
				m.status.Store(StatusStopping)
				publishMCPLoggingEvent(m, attempt)
				entries := observed.All()
				require.Len(t, entries, 1)
				require.Equal(t, zapcore.ErrorLevel, entries[0].Level)
				require.Equal(t, failure.Error(), entries[0].ContextMap()["error"])
			})
		}
	}
}
