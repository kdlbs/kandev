package backendapp

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/plugins"
)

type previewClient interface {
	PrepareFilePreview(context.Context, string, string) (*agentctltypes.FilePreviewMetadata, error)
	CopyFilePreview(context.Context, string, int64, io.Writer) error
	CancelFilePreview(context.Context, string) error
}
type previewExecution struct {
	id, workspace, label string
	client               previewClient
	release              func()
	validate             func(context.Context) error
}
type previewExecutionResolver func(context.Context, string) (previewExecution, error)

var errPreviewExecution = errors.New("preview execution unavailable")

func preparePluginFilePreview(ctx context.Context, resolve previewExecutionResolver, session, repo, path string) (*plugins.FilePreviewSource, error) {
	selected, err := resolve(ctx, session)
	if err != nil {
		return nil, err
	}
	meta, err := selected.client.PrepareFilePreview(ctx, path, repo)
	selected.release()
	if errors.Is(err, agentctltypes.ErrFilePreviewLimit) {
		return nil, plugins.ErrFilePreviewTooLarge
	}
	if err != nil {
		return nil, err
	}
	var once sync.Once
	source := &plugins.FilePreviewSource{WorkspaceID: selected.workspace, Path: meta.Path, ExecutionLabel: selected.label, External: meta.External, SizeBytes: meta.SizeBytes}
	source.Close = func() {
		once.Do(func() {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = selected.client.CancelFilePreview(cancelCtx, meta.Token)
		})
	}
	source.Validate = func(ctx context.Context) error {
		if selected.validate != nil {
			return selected.validate(ctx)
		}
		return ctx.Err()
	}
	source.Read = func(ctx context.Context, w io.Writer) error {
		current, err := resolve(ctx, session)
		if err != nil {
			return err
		}
		if current.id != selected.id || current.client != selected.client || current.workspace != selected.workspace {
			current.release()
			return errPreviewExecution
		}
		err = current.client.CopyFilePreview(ctx, meta.Token, meta.SizeBytes, w)
		current.release()
		if err != nil {
			return err
		}
		final, err := resolve(ctx, session)
		if err != nil {
			return err
		}
		defer final.release()
		if final.id != selected.id || final.client != selected.client || final.workspace != selected.workspace {
			return errPreviewExecution
		}
		return nil
	}
	return source, nil
}

func wirePluginFilePreviews(p routeParams) {
	if p.lifecycleMgr == nil || p.services.Task == nil {
		return
	}
	resolve := func(ctx context.Context, session string) (previewExecution, error) {
		if err := p.services.Task.AuthorizeSessionAccess(ctx, session); err != nil {
			return previewExecution{}, err
		}
		execution, err := p.lifecycleMgr.GetOrEnsureExecution(ctx, session)
		if err != nil || execution == nil {
			return previewExecution{}, errPreviewExecution
		}
		client, release := execution.AcquireAgentCtlClient()
		if client == nil {
			return previewExecution{}, errPreviewExecution
		}
		validate := func(ctx context.Context) error {
			if err := p.services.Task.AuthorizeSessionAccess(ctx, session); err != nil {
				return err
			}
			current, exists := p.lifecycleMgr.GetExecutionBySessionID(session)
			if !exists || current != execution {
				return errPreviewExecution
			}
			return ctx.Err()
		}
		return previewExecution{id: execution.ID, workspace: execution.WorkspaceID, label: execution.ExecutorType, client: client, release: release, validate: validate}, nil
	}
	p.services.Plugins.SetFilePreviewBackend(func(ctx context.Context, session, repo, path string) (*plugins.FilePreviewSource, error) {
		return preparePluginFilePreview(ctx, resolve, session, repo, path)
	})
}
