package messagequeue

import (
	"context"
	"errors"
	"reflect"
)

const (
	MaxRecoveryEntries     = 1024
	MaxRecoverySourceBytes = 16 * 1024 * 1024
)

var ErrRecoveryLimit = errors.New("queue recovery source exceeds limit")

type recoveryReadRepository interface {
	RecoveryRead(context.Context, QueueSessionIdentity) (RepositorySnapshot, error)
}

// RecoveryRead observes an exact incarnation without binding identity, taking
// dispatch ownership, advancing status generations or changing queue policy.
func (s *Service) RecoveryRead(ctx context.Context, identity QueueSessionIdentity) (RepositorySnapshot, error) {
	repo, ok := s.repo.(recoveryReadRepository)
	if !ok {
		return RepositorySnapshot{}, errors.New("queue recovery observation unavailable")
	}
	return repo.RecoveryRead(ctx, identity)
}

func recoverySourceBytes(entry *QueuedMessage) (int, error) {
	if len(entry.Content) > MaxRecoverySourceBytes {
		return 0, ErrRecoveryLimit
	}
	baseBytes := len(entry.ID) + len(entry.TaskID) + len(entry.SessionID) + len(entry.Content) + len(entry.Model) + len(entry.QueuedBy)
	remaining := MaxRecoverySourceBytes - baseBytes
	if err := recoveryValueBound(reflect.ValueOf(entry.Attachments), &remaining, 0); err != nil {
		return 0, err
	}
	if err := recoveryValueBound(reflect.ValueOf(entry.Metadata), &remaining, 0); err != nil {
		return 0, err
	}
	attachments, err := marshalAttachments(entry.Attachments)
	if err != nil {
		return 0, err
	}
	metadata, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return 0, err
	}
	return baseBytes + len(attachments) + len(metadata), nil
}

func recoveryEntriesBytes(entries []QueuedMessage) (int, error) {
	total := 0
	for i := range entries {
		n, err := recoverySourceBytes(&entries[i])
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func (s *Service) RecoveryInventory(ctx context.Context, identities []QueueSessionIdentity, workspaceID string) ([]*QueueRecoveryManifest, error) {
	repo, ok := s.repo.(interface {
		RecoveryReadBatch(context.Context, []QueueSessionIdentity) ([]RepositorySnapshot, error)
	})
	if !ok {
		return nil, errors.New("queue recovery inventory unavailable")
	}
	snapshots, err := repo.RecoveryReadBatch(ctx, identities)
	if err != nil {
		return nil, err
	}
	result := make([]*QueueRecoveryManifest, 0, len(snapshots))
	for i, snapshot := range snapshots {
		manifest, err := recoveryManifest(identities[i], workspaceID, snapshot.Entries)
		if err != nil {
			return nil, err
		}
		result = append(result, manifest)
	}
	if err := recoveryResponseBound(result); err != nil {
		return nil, err
	}
	return result, nil
}
