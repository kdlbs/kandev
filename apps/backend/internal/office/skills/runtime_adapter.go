// Package skills hosts the office-side skill model + repo. After ADR
// 0005 Wave E the deployment logic lives in
// internal/agent/runtime/lifecycle/skill; this file exposes adapters
// that bridge office's repo and service to the runtime-tier
// SkillReader / InstructionLister interfaces.
package skills

import (
	"context"

	runtimeskill "github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	"github.com/kandev/kandev/internal/office/models"
)

// officeSkillReader wraps anything that knows how to look up an
// office Skill by id-or-slug.
type officeSkillReader interface {
	GetSkillFromConfig(ctx context.Context, idOrSlug string) (*models.Skill, error)
}

// officeInstructionLister wraps anything that lists instruction files
// for an agent_profile_id.
type officeInstructionLister interface {
	ListInstructions(ctx context.Context, agentProfileID string) ([]*models.InstructionFile, error)
}

// SkillReaderAdapter bridges an office Service / repo to the
// runtime-tier SkillReader interface used by skill.Deployer.
type SkillReaderAdapter struct {
	inner officeSkillReader
}

// NewSkillReaderAdapter wraps an office skill reader for the runtime.
func NewSkillReaderAdapter(inner officeSkillReader) *SkillReaderAdapter {
	return &SkillReaderAdapter{inner: inner}
}

// GetSkillFromConfig satisfies runtimeskill.SkillReader. Returns nil
// (no error) when the office reader is unset, which keeps the
// deployer safe to construct in environments without office wiring.
func (a *SkillReaderAdapter) GetSkillFromConfig(ctx context.Context, idOrSlug string) (*runtimeskill.Skill, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	sk, err := a.inner.GetSkillFromConfig(ctx, idOrSlug)
	if err != nil {
		return nil, err
	}
	if sk == nil {
		return nil, nil
	}
	return &runtimeskill.Skill{
		Slug:       sk.Slug,
		Content:    sk.Content,
		SourceType: string(sk.SourceType),
	}, nil
}

// InstructionListerAdapter bridges an office repo to the runtime-tier
// InstructionLister interface used by skill.Deployer.
type InstructionListerAdapter struct {
	inner officeInstructionLister
}

// NewInstructionListerAdapter wraps an office instruction lister for
// the runtime.
func NewInstructionListerAdapter(inner officeInstructionLister) *InstructionListerAdapter {
	return &InstructionListerAdapter{inner: inner}
}

// ListInstructions satisfies runtimeskill.InstructionLister. Returns
// nil when the office lister is unset.
func (a *InstructionListerAdapter) ListInstructions(ctx context.Context, agentProfileID string) ([]*runtimeskill.InstructionFile, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	files, err := a.inner.ListInstructions(ctx, agentProfileID)
	if err != nil {
		return nil, err
	}
	out := make([]*runtimeskill.InstructionFile, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		out = append(out, &runtimeskill.InstructionFile{
			Filename: f.Filename,
			Content:  f.Content,
			IsEntry:  f.IsEntry,
		})
	}
	return out, nil
}
