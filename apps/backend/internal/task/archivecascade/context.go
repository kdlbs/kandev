package archivecascade

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrSizeExceeded identifies a cascade that exceeds a bounded v1 dimension.
var ErrSizeExceeded = errors.New("archive_size_exceeded")

// SizeExceededError reports the rejected cascade dimension and observed bound.
type SizeExceededError struct {
	Dimension string
	Limit     int
	Submitted int
}

func (e *SizeExceededError) Error() string {
	return fmt.Sprintf("%s: %s limit %d submitted %d", ErrSizeExceeded, e.Dimension, e.Limit, e.Submitted)
}

func (e *SizeExceededError) Unwrap() error {
	return ErrSizeExceeded
}

// ErrCrossWorkspaceDescendant identifies a cascade containing a task outside
// the root task's workspace.
var ErrCrossWorkspaceDescendant = errors.New("cross_workspace_descendant")

// CrossWorkspaceDescendantError reports the rejected task and workspace IDs.
type CrossWorkspaceDescendantError struct {
	TaskID              string
	WorkspaceID         string
	ExpectedWorkspaceID string
}

func (e *CrossWorkspaceDescendantError) Error() string {
	return fmt.Sprintf("%s: task %s belongs to workspace %s, expected %s",
		ErrCrossWorkspaceDescendant, e.TaskID, e.WorkspaceID, e.ExpectedWorkspaceID)
}

func (e *CrossWorkspaceDescendantError) Unwrap() error {
	return ErrCrossWorkspaceDescendant
}

// ErrInvalidStructure identifies a cascade with a cycle or duplicate member.
var ErrInvalidStructure = errors.New("archive_cascade_invalid_structure")

// StructureError reports the invalid relationship and repeated task ID.
type StructureError struct {
	Kind   string
	TaskID string
}

func (e *StructureError) Error() string {
	return fmt.Sprintf("%s: %s at task %s", ErrInvalidStructure, e.Kind, e.TaskID)
}

func (e *StructureError) Unwrap() error {
	return ErrInvalidStructure
}

const TaskArchiveTimeout = 2 * time.Minute

// ArchiveDeadline captures the absolute archive deadline at operation entry.
func ArchiveDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(TaskArchiveTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	return deadline
}

// ContinuationContext preserves the caller deadline while detaching committed
// archive work from cancellation caused by stopping the caller's execution.
func ContinuationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return ContinuationContextUntil(ctx, ArchiveDeadline(ctx))
}

// ContinuationContextUntil detaches cancellation while retaining the captured
// absolute deadline for archive work and compensation.
func ContinuationContextUntil(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}
