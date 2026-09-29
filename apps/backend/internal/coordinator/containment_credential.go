package coordinator

import (
	"context"
	"strings"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// credentialScan accumulates the outcome of condition 3 over its sources.
type credentialScan struct {
	unreadable bool
	changed    bool
	unverified bool
	match      string
}

func (s *credentialScan) detail() string {
	switch {
	case s.unreadable:
		return DetailUnreadable
	case s.match != "":
		return s.match
	case s.changed:
		return DetailChangedSinceLaunch
	case s.unverified:
		return DetailUnverifiedSource
	}
	return ""
}

// definition evaluates one environment definition on its own. Only the first
// match names the detail.
func (s *credentialScan) definition(key, value string) {
	if s.match != "" {
		return
	}
	switch {
	case key == DetailKandevAPIKey:
		s.match = DetailKandevAPIKey
	case key == DetailKandevRunToken:
		s.match = DetailKandevRunToken
	case strings.HasPrefix(value, kandevPATPrefix):
		s.match = DetailPATValue
	}
}

func (s *credentialScan) source(launch launchInfo, updatedAt time.Time) {
	switch launch.currency(updatedAt) {
	case DetailUnreadable:
		s.unreadable = true
	case DetailChangedSinceLaunch:
		s.changed = true
	}
}

func (c *ContainmentChecker) noKandevCredential(ctx context.Context, co *Coordinator, launch launchInfo) ContainmentCondition {
	scan := &credentialScan{}
	if co == nil || c.readers.Executors == nil || c.readers.Agents == nil || c.readers.Secrets == nil || ctx.Err() != nil {
		scan.unreadable = true
	} else {
		c.scanExecutorProfile(ctx, co, launch, scan)
		c.scanAgentProfile(ctx, co, launch, scan)
		c.scanRepositoryBindings(ctx, co, scan)
	}
	detail := scan.detail()
	return ContainmentCondition{Name: ConditionNoKandevCredential, Met: detail == "", Detail: detail}
}

func (c *ContainmentChecker) scanExecutorProfile(ctx context.Context, co *Coordinator, launch launchInfo, scan *credentialScan) {
	profile, err := c.readers.Executors.GetExecutorProfile(ctx, co.ExecutorProfileID)
	if err != nil || profile == nil {
		scan.unreadable = true
		return
	}
	scan.source(launch, profile.UpdatedAt)
	c.scanEnvVars(ctx, profile.EnvVars, launch, scan)
}

func (c *ContainmentChecker) scanAgentProfile(ctx context.Context, co *Coordinator, launch launchInfo, scan *credentialScan) {
	profile, err := c.readers.Agents.GetAgentProfile(ctx, co.AgentProfileID)
	if err != nil || profile == nil {
		scan.unreadable = true
		return
	}
	scan.source(launch, profile.UpdatedAt)
	c.scanEnvVars(ctx, profile.EnvVars, launch, scan)
	if profile.ProviderAPIKeySecretID != "" {
		if value, ok := c.reveal(ctx, profile.ProviderAPIKeySecretID, launch, scan); ok {
			scan.definition("", value)
		}
	}
	c.scanCredentialManager(ctx, profile.AgentID, scan)
}

func (c *ContainmentChecker) scanEnvVars(ctx context.Context, vars []taskmodels.ProfileEnvVar, launch launchInfo, scan *credentialScan) {
	for _, ev := range vars {
		if ev.Key == "" {
			continue
		}
		value := ev.Value
		if ev.SecretID != "" {
			revealed, ok := c.reveal(ctx, ev.SecretID, launch, scan)
			if !ok {
				continue
			}
			value = revealed
		}
		scan.definition(ev.Key, value)
	}
}

func (c *ContainmentChecker) reveal(ctx context.Context, secretID string, launch launchInfo, scan *credentialScan) (string, bool) {
	value, updatedAt, err := c.readers.Secrets.RevealSecret(ctx, secretID)
	if err != nil {
		scan.unreadable = true
		return "", false
	}
	scan.source(launch, updatedAt)
	return value, true
}

func (c *ContainmentChecker) scanCredentialManager(ctx context.Context, agentID string, scan *credentialScan) {
	if c.readers.Credentials == nil {
		scan.unverified = true
		return
	}
	values, err := c.readers.Credentials.CredentialValues(ctx, agentID)
	if err != nil {
		scan.unreadable = true
		return
	}
	for _, v := range values {
		scan.definition(v.Key, v.Value)
	}
}

func (c *ContainmentChecker) scanRepositoryBindings(ctx context.Context, co *Coordinator, scan *credentialScan) {
	if co.ConversationTaskID == nil || *co.ConversationTaskID == "" {
		return
	}
	if c.readers.Repositories == nil {
		scan.unverified = true
		return
	}
	bound, err := c.readers.Repositories.HasRepositoryBinding(ctx, *co.ConversationTaskID)
	if err != nil {
		scan.unreadable = true
		return
	}
	if bound {
		scan.unverified = true
	}
}
