package process

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/journal"

	"go.uber.org/zap"
)

func (m *Manager) logMCPAttachmentCommitError(message string, err error) {
	if m.expectedMCPAttachmentShutdownError(err) {
		m.logger.Debug(message, zap.Error(err))
		return
	}
	m.logger.Error(message, zap.Error(err))
}

func (m *Manager) expectedMCPAttachmentShutdownError(err error) bool {
	status := m.Status()
	if status != StatusStopping && status != StatusStopped {
		return false
	}
	if errors.Is(err, errDeliveryWriterClosed) {
		return true
	}
	if m.lifetimeCtx == nil || m.lifetimeCtx.Err() != context.Canceled {
		return false
	}
	if err == context.Canceled {
		return true
	}
	if err != journal.ErrJournalClosed {
		return false
	}
	m.deliveryJournalMu.RLock()
	defer m.deliveryJournalMu.RUnlock()
	return m.deliveryJournal == nil && m.deliveryJournalErr == journal.ErrJournalClosed && m.deliveryJournalCloseErr == nil
}
