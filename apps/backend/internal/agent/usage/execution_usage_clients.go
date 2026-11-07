package usage

import (
	"context"
	"fmt"
)

type executionAccountClient interface {
	ProviderUsageClient
	accountIdentity(context.Context) (executionAccount, error)
	matchesSource(executionUsageSource) bool
}

type codexExecutionUsageClient struct{ base *CodexUsageClient }

func (c *codexExecutionUsageClient) matchesSource(source executionUsageSource) bool {
	return source.provider == executionProviderOpenAI && source.path == c.base.authPath && source.token == ""
}

func (c *codexExecutionUsageClient) accountIdentity(context.Context) (executionAccount, error) {
	auth, err := c.base.readAuth()
	if err != nil {
		return executionAccount{}, err
	}
	if auth.Tokens == nil || auth.Tokens.AccessToken == "" || auth.Tokens.AccountID == "" {
		return executionAccount{}, fmt.Errorf("execution subscription account is unavailable")
	}
	return executionAccount{account: auth.Tokens.AccountID}, nil
}

func (c *codexExecutionUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	sample, err := c.base.FetchUsage(ctx)
	if sample != nil {
		for index := range sample.Windows {
			window := &sample.Windows[index]
			if window.Observation != nil && window.UtilizationPct > 100 && nativeUtilizationObserved(&window.UtilizationPct) {
				window.Observation.UtilizationObserved = true
			}
		}
	}
	return sample, err
}

func newExecutionAccountClient(source executionUsageSource) executionAccountClient {
	if source.provider == executionProviderOpenAI {
		return &codexExecutionUsageClient{base: NewCodexUsageClientWithPath(source.path)}
	}
	return &claudeExecutionUsageClient{
		base: NewClaudeUsageClientWithPath(source.path), directToken: source.token,
		profileURL: "https://api.anthropic.com/api/oauth/profile",
	}
}
