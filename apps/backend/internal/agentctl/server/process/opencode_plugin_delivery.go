package process

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/opencodeplugin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

var errOpenCodePluginStaleAdmission = opencodeplugin.ErrAdmissionRevoked

type openCodePluginClaimSource interface {
	OpenCodePluginClaims() []opencodeplugin.Claim
}

// openCodePluginAdmissionValidator is the adapter-owned native generation and
// claim fence. It is checked under the host's admission fence immediately before
// the journal write, so a session transition cannot leave a stale acceptance
// window while ordinary adapter updates are still queued.
type openCodePluginAdmissionValidator interface {
	ValidateOpenCodePluginAdmission(admission opencodeplugin.Admission) bool
}

type openCodePluginClaimRetirer interface {
	RetireOpenCodePluginClaim(admission opencodeplugin.Admission)
}

// openCodePluginAdmissionGuard holds the adapter's native-session read fence
// across the durable journal commit. A session transition or reset needs the
// write side and therefore cannot revoke ownership between validation and the
// commit. The returned release function must always be called.
type openCodePluginAdmissionGuard interface {
	BeginOpenCodePluginAdmission(admission opencodeplugin.Admission) (func(), bool)
}

// SetOpenCodePluginHost attaches the launch host owned by this process. A nil
// host disables claim publication and admission validation.
func (m *Manager) SetOpenCodePluginHost(host *opencodeplugin.Host) {
	if m == nil {
		return
	}
	m.openCodePluginMu.Lock()
	m.openCodePluginHost = host
	m.openCodePluginValidator = nil
	m.openCodePluginGuard = nil
	m.openCodePluginRetirer = nil
	if host != nil {
		if validator, ok := m.adapter.(openCodePluginAdmissionValidator); ok {
			m.openCodePluginValidator = validator
		}
		if guard, ok := m.adapter.(openCodePluginAdmissionGuard); ok {
			m.openCodePluginGuard = guard
		}
		if retirer, ok := m.adapter.(openCodePluginClaimRetirer); ok {
			m.openCodePluginRetirer = retirer
		}
	}
	m.openCodePluginMu.Unlock()
}

func (m *Manager) openCodePlugin() *opencodeplugin.Host {
	if m == nil {
		return nil
	}
	m.openCodePluginMu.Lock()
	defer m.openCodePluginMu.Unlock()
	return m.openCodePluginHost
}

func (m *Manager) openCodePluginAdmissionFence() (openCodePluginAdmissionValidator, openCodePluginAdmissionGuard) {
	m.openCodePluginMu.Lock()
	defer m.openCodePluginMu.Unlock()
	return m.openCodePluginValidator, m.openCodePluginGuard
}

// syncOpenCodePluginClaims publishes the adapter's immutable launch snapshot as
// complete shell-outcomes configuration. Submission attribution comes from the
// claim's captured prompt generation, never from the active submission.
func (m *Manager) syncOpenCodePluginClaims(agentAdapter adapter.AgentAdapter) {
	host := m.openCodePlugin()
	source, ok := agentAdapter.(openCodePluginClaimSource)
	if host == nil || !ok {
		return
	}
	// Read and publish one complete snapshot under a single lock. Without this,
	// two syncs can read different generations and the older snapshot can land
	// after the newer one.
	m.openCodePluginSyncMu.Lock()
	defer m.openCodePluginSyncMu.Unlock()
	claims := source.OpenCodePluginClaims()
	for index := range claims {
		if claims[index].SubmissionID == "" {
			claims[index].SubmissionID = m.submissionForGeneration(claims[index].PromptGeneration)
		}
	}
	raw, err := opencodeplugin.MarshalShellConfig(claims)
	if err != nil {
		return
	}
	if len(claims) == 0 {
		host.RevokeBehaviorConfig(opencodeplugin.BehaviorShellOutcomes, raw)
		return
	}
	host.SetBehaviorConfig(opencodeplugin.BehaviorShellOutcomes, raw)
}

// CommitOpenCodePluginObservation is the plugin host's commit-before-ack
// boundary. It joins the ordinary ordered delivery journal and publishes only
// after a durable commit. It never creates a turn or completes a submission.
func (m *Manager) CommitOpenCodePluginObservation(ctx context.Context, admission opencodeplugin.Admission) (func(), error) {
	if m == nil || strings.TrimSpace(admission.EventID) == "" {
		return nil, context.Canceled
	}
	release, err := m.beginOpenCodePluginAdmission(admission)
	if err != nil {
		return nil, err
	}
	if admission.Check != nil && !admission.Check() {
		if release != nil {
			release()
		}
		return nil, opencodeplugin.ErrAdmissionRevoked
	}
	observation := admission.Observation
	if observation.SessionID == "" {
		observation.SessionID = m.cfg.SessionID
	}
	submissionID := admission.SubmissionID
	if submissionID == "" {
		submissionID = m.submissionForGeneration(admission.PromptGeneration)
	}
	event := adapter.AgentEvent{
		Type:                 streams.EventTypeBackgroundWorkUpdated,
		SessionID:            observation.SessionID,
		BackgroundWork:       &observation,
		DeliverySubmissionID: submissionID,
		PromptGeneration:     admission.PromptGeneration,
		DeliveryEventID:      admission.EventID,
	}
	persisted, err := m.persistDeliveryEventContext(ctx, event)
	if release != nil {
		release()
	}
	if err != nil {
		return nil, err
	}
	m.openCodePluginMu.Lock()
	retirer := m.openCodePluginRetirer
	m.openCodePluginMu.Unlock()
	if retirer != nil {
		retirer.RetireOpenCodePluginClaim(admission)
	}
	return func() {
		if m.adapter != nil {
			m.syncOpenCodePluginClaims(m.adapter)
		}
		m.sendUpdateBlockingRecorded(persisted)
	}, nil
}

// beginOpenCodePluginAdmission acquires the adapter native-session fence or
// validates the legacy point-in-time fence. The returned release must run after
// the journal write.
func (m *Manager) beginOpenCodePluginAdmission(admission opencodeplugin.Admission) (func(), error) {
	validator, guard := m.openCodePluginAdmissionFence()
	if guard != nil {
		release, ok := guard.BeginOpenCodePluginAdmission(admission)
		if !ok {
			return nil, errOpenCodePluginStaleAdmission
		}
		return release, nil
	}
	if validator != nil {
		if !validator.ValidateOpenCodePluginAdmission(admission) {
			return nil, errOpenCodePluginStaleAdmission
		}
		return nil, nil
	}
	if host := m.openCodePlugin(); host != nil {
		return nil, errOpenCodePluginStaleAdmission
	}
	return nil, nil
}
