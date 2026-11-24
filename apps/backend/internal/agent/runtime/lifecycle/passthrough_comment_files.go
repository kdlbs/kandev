package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	shared "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (m *Manager) preparePassthroughCommentPrompt(ctx context.Context, execution *AgentExecution, prompt string) (string, error) {
	files := convertAttachments(getAttachmentsFromMetadata(execution))
	if !v1.HasCommentAttachments(files) {
		return prompt, nil
	}
	if err := v1.ValidateCommentAttachments(prompt, files); err != nil {
		return "", err
	}
	if isSSHPassthrough(execution) {
		return m.prepareRemotePassthroughCommentPrompt(ctx, execution, prompt, files)
	}
	if m.sessionManager == nil || m.sessionManager.attachmentReader == nil {
		return "", errors.New("attachment storage is unavailable")
	}
	materializer := shared.NewAttachmentManager(execution.WorkspacePath, m.logger.Zap())
	materializer.SetSessionID(execution.SessionID)
	paths := make([]string, 0, len(files))
	for i, file := range files {
		if file.CommentID == "" {
			continue
		}
		saved, err := materializeInitialPassthroughCommentFile(
			ctx, execution, m.sessionManager.attachmentReader, materializer, &files[i],
		)
		if saved.AbsPath != "" {
			paths = append(paths, saved.AbsPath)
		}
		if err != nil {
			for _, path := range paths {
				_ = os.Remove(path)
			}
			return "", err
		}
	}
	rendered, _, err := v1.RenderCommentFiles(prompt, files)
	return rendered, err
}

func materializeInitialPassthroughCommentFile(
	ctx context.Context,
	execution *AgentExecution,
	source AttachmentReader,
	materializer *shared.AttachmentManager,
	file *v1.MessageAttachment,
) (shared.SavedAttachment, error) {
	reader, name, mimeType, size, err := source.OpenClaimed(ctx, file.AttachmentID, execution.TaskID, execution.SessionID)
	if err != nil {
		return shared.SavedAttachment{}, fmt.Errorf("open claimed comment file: %w", err)
	}
	file.Name, file.MimeType, file.SizeBytes = name, mimeType, size
	saved, saveErr := materializer.SaveAttachmentStream(*file, reader)
	closeErr := reader.Close()
	file.Path = saved.AbsPath
	return saved, errors.Join(saveErr, closeErr)
}

// PrepareRemotePassthroughCommentPrompt uses the active SSH execution's file
// endpoint. The caller uses its local workspace writer only when handled is false.
func (m *Manager) PrepareRemotePassthroughCommentPrompt(
	ctx context.Context, sessionID, prompt string, files []v1.MessageAttachment,
) (rendered string, handled bool, err error) {
	execution, ok := m.GetExecutionBySessionID(sessionID)
	if !ok || execution == nil || !isSSHPassthrough(execution) {
		return "", false, nil
	}
	rendered, err = m.prepareRemotePassthroughCommentPrompt(ctx, execution, prompt, files)
	if err == nil {
		err = m.verifyAnswerAttachmentExecutionCurrent(execution.ID, execution.TaskID, sessionID)
	}
	return rendered, true, err
}

func (m *Manager) prepareRemotePassthroughCommentPrompt(
	ctx context.Context, execution *AgentExecution, prompt string, files []v1.MessageAttachment,
) (string, error) {
	if err := v1.ValidateCommentAttachments(prompt, files); err != nil {
		return "", err
	}
	if m.sessionManager == nil {
		return "", errors.New("attachment storage is unavailable")
	}
	materialized, err := m.sessionManager.materializeAttachmentsForSession(ctx, execution, execution.SessionID, files)
	if err != nil {
		return "", err
	}
	rendered, ordinary, err := v1.RenderCommentFiles(prompt, materialized)
	if err != nil {
		return "", err
	}
	if len(ordinary) == 0 {
		return rendered, nil
	}
	saved := make([]shared.SavedAttachment, 0, len(ordinary))
	for _, file := range ordinary {
		if file.Path == "" {
			return "", errors.New("an SSH terminal attachment has no execution path")
		}
		saved = append(saved, shared.SavedAttachment{Name: file.Name, RelPath: file.Path})
	}
	return rendered + "\n\n" + strings.TrimSpace(shared.BuildAttachmentPrompt(saved, true)), nil
}
