package intent

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"slices"
)

type bindingKey struct{}
type Binding struct {
	Intent                         Intent
	TaskID, SessionID, ExecutionID string
}

// Bind records server-validated request authority for one dispatch target.
func Bind(ctx context.Context, value Binding) context.Context {
	return context.WithValue(ctx, bindingKey{}, value)
}

func FromContext(ctx context.Context) (Binding, bool) {
	value, ok := ctx.Value(bindingKey{}).(Binding)
	return value, ok
}

func ValidateExecution(ctx context.Context, taskID, sessionID, executionID string, manifest *p.BuiltinManifest) error {
	value, exists := FromContext(ctx)
	if !exists {
		return nil
	}
	if err := value.Intent.Validate(); err != nil {
		return err
	}
	if value.Intent == "" || value.TaskID != taskID || value.SessionID != sessionID || executionID == "" {
		return ErrUnavailable
	}
	if value.ExecutionID != "" && value.ExecutionID != executionID {
		return ErrUnavailable
	}
	if manifest == nil {
		if value.ExecutionID != "" {
			return nil
		}
		return ErrUnavailable
	}
	if manifest.Validate() != nil || manifest.TaskID != taskID || !slices.Contains(manifest.Packages, p.PackageWalkthroughs) {
		return ErrUnavailable
	}
	return nil
}
