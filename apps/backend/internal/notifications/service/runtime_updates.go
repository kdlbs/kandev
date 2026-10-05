package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/notifications/models"
	"go.uber.org/zap"
)

func (s *Service) HandleAgentRuntimeUpdate(ctx context.Context, notice agents.RuntimeUpdateNotice) {
	if notice.AgentID == "" || notice.RuntimeID == "" || notice.Version == "" || notice.OccurrenceID == "" {
		return
	}
	s.handleSemanticOccurrence(ctx, "", "", notice.OccurrenceID, EventSystemUpdateAvailable, map[string]string{
		"agent_name": notice.AgentID, "runtime_id": notice.RuntimeID, "display_name": notice.DisplayName,
		"previous_version": notice.PreviousVersion, "version": notice.Version, "runtime_update_status": notice.Status, "url": notice.URL,
	})
}

// HandleAgentRuntimeUpdates delivers available versions as one occurrence per
// recipient and provider while retaining each runtime's durable delivery claim.
func (s *Service) HandleAgentRuntimeUpdates(ctx context.Context, notices []agents.RuntimeUpdateNotice) {
	valid := uniqueAvailableRuntimeNotices(notices)
	if len(valid) == 0 {
		return
	}
	recipients, ok := s.recipients(ctx, "", EventSystemUpdateAvailable)
	if !ok {
		return
	}
	for _, userID := range recipients {
		s.deliverRuntimeUpdateBatch(ctx, userID, valid)
	}
}

func uniqueAvailableRuntimeNotices(notices []agents.RuntimeUpdateNotice) []agents.RuntimeUpdateNotice {
	ordered := append([]agents.RuntimeUpdateNotice(nil), notices...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].AgentID != ordered[j].AgentID {
			return ordered[i].AgentID < ordered[j].AgentID
		}
		if ordered[i].RuntimeID != ordered[j].RuntimeID {
			return ordered[i].RuntimeID < ordered[j].RuntimeID
		}
		return ordered[i].OccurrenceID < ordered[j].OccurrenceID
	})
	unique := make([]agents.RuntimeUpdateNotice, 0, len(ordered))
	seen := make(map[string]struct{}, len(ordered))
	for _, notice := range ordered {
		if notice.AgentID == "" || notice.RuntimeID == "" || notice.Version == "" || notice.OccurrenceID == "" || notice.Status != "available" {
			continue
		}
		key := notice.AgentID + "\x00" + notice.RuntimeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, notice)
	}
	return unique
}

func (s *Service) deliverRuntimeUpdateBatch(ctx context.Context, userID string, notices []agents.RuntimeUpdateNotice) {
	providerList, subscriptions, err := s.ListProviders(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load notification providers", zap.Error(err))
		return
	}
	for _, provider := range providerList {
		if !provider.Enabled || !containsEvent(subscriptions[provider.ID], EventSystemUpdateAvailable) {
			continue
		}
		claimed := make([]agents.RuntimeUpdateNotice, 0, len(notices))
		for _, notice := range notices {
			inserted, err := s.repo.InsertDelivery(ctx, &models.Delivery{
				UserID: userID, ProviderID: provider.ID, EventType: EventSystemUpdateAvailable,
				OccurrenceID: notice.OccurrenceID,
			})
			if err != nil {
				s.logger.Error("failed to record runtime update delivery", zap.String("provider_id", provider.ID), zap.Error(err))
				continue
			}
			if inserted {
				claimed = append(claimed, notice)
			}
		}
		if len(claimed) == 0 {
			continue
		}
		message := runtimeUpdateBatchMessage(claimed)
		if err := s.dispatchProvider(ctx, userID, provider, message); err != nil {
			s.logger.Warn("runtime update notification delivery failed", zap.String("provider_id", provider.ID), zap.Error(err))
			rollbackCtx := context.WithoutCancel(ctx)
			for _, notice := range claimed {
				if rollbackErr := s.repo.DeleteDelivery(rollbackCtx, provider.ID, EventSystemUpdateAvailable, notice.OccurrenceID); rollbackErr != nil {
					s.logger.Error("failed to release runtime update delivery claim", zap.String("provider_id", provider.ID), zap.Error(rollbackErr))
				}
			}
		}
	}
}

func runtimeUpdateBatchMessage(notices []agents.RuntimeUpdateNotice) notificationPayload {
	if len(notices) == 1 {
		notice := notices[0]
		payload := map[string]string{
			"agent_name": notice.AgentID, "runtime_id": notice.RuntimeID, "display_name": notice.DisplayName,
			"previous_version": notice.PreviousVersion, "version": notice.Version,
			"runtime_update_status": notice.Status, "url": notice.URL,
		}
		title, body := runtimeUpdateMessage(payload)
		return notificationPayload{
			OccurrenceID: notice.OccurrenceID, EventType: EventSystemUpdateAvailable,
			Title: title, Body: body, Payload: payload,
		}
	}
	members := make([]models.RuntimeUpdateMember, 0, len(notices))
	occurrences := make([]string, 0, len(notices))
	for _, notice := range notices {
		members = append(members, models.RuntimeUpdateMember{
			OccurrenceID: notice.OccurrenceID, AgentName: notice.AgentID, RuntimeID: notice.RuntimeID,
			DisplayName: notice.DisplayName, PreviousVersion: notice.PreviousVersion, Version: notice.Version,
		})
		occurrences = append(occurrences, notice.OccurrenceID)
	}
	sort.Strings(occurrences)
	summaryKey := sha256.Sum256([]byte(strings.Join(occurrences, "\x00")))
	return notificationPayload{
		OccurrenceID: fmt.Sprintf("agent-runtime-summary:%x", summaryKey),
		EventType:    EventSystemUpdateAvailable,
		Title:        fmt.Sprintf("%d agent runtime updates available", len(members)),
		Body:         "Review runtime versions in Settings > Agents.",
		Payload: map[string]string{
			"notification_kind": "agent_runtime_summary",
			"url":               "/settings/agents#runtime-updates",
		},
		RuntimeUpdates: members,
	}
}

func runtimeUpdateMessage(payload map[string]string) (string, string) {
	name, previous, target := payload["display_name"], payload["previous_version"], payload["version"]
	switch payload["runtime_update_status"] {
	case "succeeded":
		return fmt.Sprintf("%s runtime updated", name), fmt.Sprintf("%s runtime updated from %s to %s for future sessions. Open Settings > Agents > Runtime updates to select another version or return to the Kandev default.", name, previous, target)
	case "interrupted":
		return fmt.Sprintf("%s runtime update interrupted", name), fmt.Sprintf("%s runtime update from %s to %s could not be confirmed. Open Settings > Agents > Runtime updates to review the current selection before retrying or returning to the Kandev default.", name, previous, target)
	case "failed":
		return fmt.Sprintf("%s runtime update failed", name), fmt.Sprintf("%s runtime update from %s to %s did not activate. This attempt preserved your selection. Open Settings > Agents > Runtime updates to retry manually or disable automatic updates.", name, previous, target)
	default:
		return fmt.Sprintf("%s runtime update available", name), fmt.Sprintf("%s runtime %s is available (selected version: %s). Open Settings > Agents > Runtime updates to review the update control or manual guidance.", name, target, previous)
	}
}
