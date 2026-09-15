package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"time"
)

func (s *Service) AcquireAutomationAdapter(id, key string) (manifest.AutomationCondition, string, pluginsdk.AutomationAdapter, func(), error) {
	empty := manifest.AutomationCondition{}
	rec, err := s.Get(id)
	if err != nil {
		return empty, "", nil, nil, err
	}
	rec, release, err := s.beginPluginDispatch(id, dispatchGeneration(rec))
	if err != nil {
		return empty, "", nil, nil, err
	}
	remote, ok := s.pluginRemote(id)
	if ok {
		for _, c := range rec.AutomationConditions {
			if c.Key == key && c.Access == "public" {
				return c, rec.Version + ":" + rec.InstalledAt.UTC().Format(time.RFC3339Nano), remote, release, nil
			}
		}
	}
	release()
	return empty, "", nil, nil, fmt.Errorf("automation adapter unavailable")
}
func (s *Service) AutomationConditions(ctx context.Context, workspaceID string) []automation.PluginConditionInfo {
	result := []automation.PluginConditionInfo{}
	for _, rec := range s.List() {
		if rec.Status != StatusActive {
			continue
		}
		for _, c := range rec.AutomationConditions {
			info := automation.PluginConditionInfo{PluginID: rec.ID, ProviderLabel: rec.DisplayName, Condition: c}
			_, _, adapter, release, err := s.AcquireAutomationAdapter(rec.ID, c.Key)
			if err == nil {
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				resp, callErr := adapter.DescribeAutomationCondition(callCtx, &pluginsdk.AutomationConditionRequest{WorkspaceId: workspaceID, ConditionKey: c.Key})
				cancel()
				release()
				if callErr == nil && resp != nil {
					info.ConfigOptions = automationFieldOptions(resp.ConfigOptions, c)
					info.Available = resp.Available
					info.Reason = resp.Reason
				}
			}
			result = append(result, info)
		}
	}
	return result
}
func (s *Service) SetAutomationSecret(ctx context.Context, id, value string) error {
	if s.secrets == nil {
		return fmt.Errorf("secret vault unavailable")
	}
	return s.secrets.Set(ctx, id, "Automation webhook", value)
}
func (s *Service) ReadAutomationSecret(ctx context.Context, id string) (string, error) {
	if s.secrets == nil {
		return "", fmt.Errorf("secret vault unavailable")
	}
	return s.secrets.Reveal(ctx, id)
}
func (s *Service) DeleteAutomationSecret(ctx context.Context, id string) error {
	if s.secrets == nil {
		return fmt.Errorf("secret vault unavailable")
	}
	return s.secrets.Delete(ctx, id)
}

func (s *Service) SetAutomationRevoker(revoke func(string) error) { s.automationRevoker = revoke }
func (s *Service) cancelAutomationDeliveries(id string) error {
	if s.automationRevoker == nil {
		return nil
	}
	return s.automationRevoker(id)
}

func automationFieldOptions(raw []byte, c manifest.AutomationCondition) map[string][]string {
	options := map[string][]string{}
	if len(raw) > 65536 || json.Unmarshal(raw, &options) != nil {
		return nil
	}
	props, _ := c.ConfigSchema["properties"].(map[string]any)
	for key, values := range options {
		prop, _ := props[key].(map[string]any)
		if prop["type"] != "string" || len(values) > 100 {
			return nil
		}
		for _, value := range values {
			if len(value) > 4096 {
				return nil
			}
		}
	}
	return options
}
