package lsp

import (
	"context"
	"time"
)

// Store persists task/language policy and evidence without introducing a
// session, browser, editor, or execution ownership key.
type Store interface {
	GetTaskLSPLanguage(ctx context.Context, taskID, language string) (*TaskLanguageState, bool, error)
	ListTaskLSPLanguages(ctx context.Context, taskID string) ([]TaskLanguageState, error)
	ListAllTaskLSPLanguages(ctx context.Context) ([]TaskLanguageState, error)
	CompareAndUpdateTaskLSPLanguage(
		ctx context.Context,
		next TaskLanguageState,
		expectedRevision uint64,
	) (*TaskLanguageState, error)
	AllocateTaskLSPGeneration(
		ctx context.Context,
		taskID, language string,
		action Action,
		initiator Initiator,
		reason string,
		acceptedAt time.Time,
	) (*TaskLanguageState, error)
}
