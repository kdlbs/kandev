package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type configurationCommandResultContextKey struct{}

type configurationCommandTurnCompleter interface {
	CompleteTurnWithConfigurationResult(context.Context, string, *streams.ConfigurationCommandResult) error
}

func withConfigurationCommandResult(
	ctx context.Context,
	result *streams.ConfigurationCommandResult,
) context.Context {
	if result == nil {
		return ctx
	}
	return context.WithValue(ctx, configurationCommandResultContextKey{}, result)
}

func configurationCommandResultFromContext(ctx context.Context) *streams.ConfigurationCommandResult {
	result, _ := ctx.Value(configurationCommandResultContextKey{}).(*streams.ConfigurationCommandResult)
	return result
}
