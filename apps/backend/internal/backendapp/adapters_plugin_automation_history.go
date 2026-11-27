package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/sysprompt"
)

type automationHistoryAdapter struct{ service *automation.Service }

func (a automationHistoryAdapter) ListAutomationHistoryDefinitions(
	ctx context.Context, workspaceID, automationID string, createdFrom, createdBefore *time.Time, limit int,
) (plugins.AutomationHistoryDefinitionList, error) {
	result, err := a.service.ListHistoryAutomations(ctx, automation.HistoryDefinitionFilter{
		WorkspaceID: workspaceID, AutomationID: automationID,
		CreatedFrom: createdFrom, CreatedBefore: createdBefore, Limit: limit,
	})
	if err != nil {
		return plugins.AutomationHistoryDefinitionList{}, adaptAutomationHistoryError(err)
	}
	items := make([]plugins.AutomationHistoryDefinition, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, adaptAutomationHistoryDefinition(item))
	}
	return plugins.AutomationHistoryDefinitionList{Items: items, Overflow: result.Overflow}, nil
}

func (a automationHistoryAdapter) ListAutomationHistoryRuns(
	ctx context.Context, workspaceID, automationID string, createdFrom, createdBefore *time.Time, limit int,
) (plugins.AutomationHistoryRunList, error) {
	result, err := a.service.ListHistoryRuns(ctx, automation.HistoryRunFilter{
		WorkspaceID: workspaceID, AutomationID: automationID,
		CreatedFrom: createdFrom, CreatedBefore: createdBefore, Limit: limit,
	})
	if err != nil {
		return plugins.AutomationHistoryRunList{}, adaptAutomationHistoryError(err)
	}
	items := make([]plugins.AutomationHistoryRun, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, adaptAutomationHistoryRun(item))
	}
	return plugins.AutomationHistoryRunList{Items: items, Overflow: result.Overflow}, nil
}

func (a automationHistoryAdapter) GetAutomationHistoryRun(
	ctx context.Context, workspaceID, runID string,
) (plugins.AutomationHistoryRun, error) {
	item, err := a.service.GetHistoryRun(ctx, workspaceID, runID)
	if err != nil {
		return plugins.AutomationHistoryRun{}, adaptAutomationHistoryError(err)
	}
	return adaptAutomationHistoryRun(*item), nil
}

func adaptAutomationHistoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, automation.ErrHistoryAutomationNotFound), errors.Is(err, automation.ErrHistoryRunNotFound):
		return plugins.ErrAutomationHistoryNotFound
	default:
		return err
	}
}

func adaptAutomationHistoryDefinition(item automation.HistoryDefinition) plugins.AutomationHistoryDefinition {
	definition := plugins.AutomationHistoryDefinition{
		ID: item.ID, WorkspaceID: item.WorkspaceID, Name: item.Name, Enabled: item.Enabled,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Triggers: make([]plugins.AutomationHistoryTrigger, 0, len(item.Triggers)),
	}
	for _, trigger := range item.Triggers {
		definition.Triggers = append(definition.Triggers, plugins.AutomationHistoryTrigger{
			ID: trigger.ID, Kind: string(trigger.Kind), Enabled: trigger.Enabled,
			CronExpression: trigger.CronExpression, Timezone: trigger.Timezone,
			ScheduleAvailable: trigger.ScheduleAvailable,
		})
	}
	definition.ResourceVersion = publicValueDigest(definition)
	return definition
}

func adaptAutomationHistoryRun(item automation.HistoryRun) plugins.AutomationHistoryRun {
	run := plugins.AutomationHistoryRun{
		ID: item.ID, WorkspaceID: item.WorkspaceID, AutomationID: item.AutomationID,
		AutomationName: item.AutomationName, TriggerID: item.TriggerID, TriggerKind: string(item.TriggerKind),
		TaskID: item.TaskID, SessionID: item.SessionID, TurnID: item.TurnID,
		ManagedConversationID: item.ManagedConversationID,
		SourceStatus:          string(item.SourceStatus), Status: string(item.Status),
		RetryState: string(item.RetryState), DeliveryStatus: string(item.DeliveryStatus),
		RetryGroupID: item.RetryGroupID, RetryParentRunID: item.RetryParentRunID,
		AttemptNumber:    item.AttemptNumber,
		CreatedAt:        item.CreatedAt,
		RetryScheduledAt: item.RetryScheduledAt, RetryClaimedAt: item.RetryClaimedAt,
		Summary:      sanitizeAutomationHistoryExcerpt(item.Summary),
		ErrorExcerpt: sanitizeAutomationHistoryExcerpt(item.ErrorMessage),
	}
	run.ResourceVersion = publicValueDigest(run)
	return run
}

func sanitizeAutomationHistoryExcerpt(value string) string {
	stripped := sysprompt.StripSystemContent(value)
	// A source excerpt may cut a system block before its closing tag. Strip
	// everything from the first remaining tag so hidden context never leaks
	// just because a bounded prefix ended mid-block.
	if cut := firstSystemTagIndex(stripped); cut >= 0 {
		stripped = stripped[:cut]
	}
	return maskTruncatedCredentialTail(securityutil.SanitizeCredentials(stripped))
}

type truncatedCredentialPrefix struct {
	prefix    string
	minimum   int
	allowPunc bool
}

var truncatedCredentialPrefixes = []truncatedCredentialPrefix{
	{prefix: "github_pat_", minimum: 50},
	{prefix: "ghp_", minimum: 30},
	{prefix: "sk-", minimum: 12, allowPunc: true},
	{prefix: "Bearer ", minimum: 20, allowPunc: true},
}

// maskTruncatedCredentialTail closes the one gap left by a bounded source
// prefix: a credential can be cut before it is long enough for the normal
// redaction pattern to match. Complete credentials were already removed, so
// only a short credential-shaped suffix is replaced here.
func maskTruncatedCredentialTail(value string) string {
	lower := strings.ToLower(value)
	for _, candidate := range truncatedCredentialPrefixes {
		index := strings.LastIndex(lower, strings.ToLower(candidate.prefix))
		if index < 0 {
			continue
		}
		if index > 0 && isCredentialTailByte(value[index-1], candidate.allowPunc) {
			continue
		}
		tail := value[index+len(candidate.prefix):]
		if len(tail) >= candidate.minimum {
			continue
		}
		valid := true
		for i := 0; i < len(tail); i++ {
			if !isCredentialTailByte(tail[i], candidate.allowPunc) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		return value[:index] + candidate.prefix + "[redacted]"
	}
	return value
}

func isCredentialTailByte(value byte, allowPunc bool) bool {
	if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' {
		return true
	}
	return allowPunc && (value == '-' || value == '.' || value == '+' || value == '/')
}

func firstSystemTagIndex(value string) int {
	start := strings.Index(value, sysprompt.TagStart)
	end := strings.Index(value, sysprompt.TagEnd)
	switch {
	case start < 0:
		return end
	case end < 0:
		return start
	case start < end:
		return start
	default:
		return end
	}
}

func publicValueDigest(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
