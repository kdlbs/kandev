package process

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/npmresolution"
)

const managedStartupEvidenceTimeout = 2 * time.Second

func (m *Manager) beginManagedStartupGeneration() uint64 {
	m.startupEvidenceMu.Lock()
	defer m.startupEvidenceMu.Unlock()
	m.startupGeneration++
	m.startupEvidence = nil
	m.startupEvidenceDone = make(chan struct{})
	return m.startupGeneration
}

// ProcessGeneration returns the generation of the current child process.
func (m *Manager) ProcessGeneration() uint64 {
	m.startupEvidenceMu.Lock()
	defer m.startupEvidenceMu.Unlock()
	return m.startupGeneration
}

// ManagedStartupEvidence waits briefly for the current child exit record and
// returns it only when the caller names the exact process generation.
func (m *Manager) ManagedStartupEvidence(ctx context.Context, generation uint64) *types.ManagedStartupEvidence {
	if generation == 0 {
		return nil
	}
	m.startupEvidenceMu.Lock()
	if generation != m.startupGeneration {
		m.startupEvidenceMu.Unlock()
		return nil
	}
	evidence := cloneManagedStartupEvidence(m.startupEvidence)
	done := m.startupEvidenceDone
	m.startupEvidenceMu.Unlock()
	if evidence != nil || done == nil {
		return evidence
	}

	timer := time.NewTimer(managedStartupEvidenceTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-ctx.Done():
		return nil
	case <-timer.C:
		return nil
	}
	m.startupEvidenceMu.Lock()
	defer m.startupEvidenceMu.Unlock()
	if generation != m.startupGeneration {
		return nil
	}
	return cloneManagedStartupEvidence(m.startupEvidence)
}

func (m *Manager) recordManagedStartupEvidence(evidence *types.ManagedStartupEvidence) {
	m.startupEvidenceMu.Lock()
	defer m.startupEvidenceMu.Unlock()
	if evidence == nil || evidence.ProcessGeneration != m.startupGeneration || m.startupEvidenceDone == nil {
		return
	}
	m.startupEvidence = cloneManagedStartupEvidence(evidence)
	close(m.startupEvidenceDone)
	m.startupEvidenceDone = nil
}

func cloneManagedStartupEvidence(evidence *types.ManagedStartupEvidence) *types.ManagedStartupEvidence {
	if evidence == nil {
		return nil
	}
	cloned := *evidence
	if evidence.ExitCode != nil {
		code := *evidence.ExitCode
		cloned.ExitCode = &code
	}
	return &cloned
}

func newManagedStartupEvidence(
	generation uint64,
	exitErr error,
	intentionalStop bool,
	collectionComplete bool,
	stderrRetainedComplete bool,
	stderr []string,
) *types.ManagedStartupEvidence {
	disposition, exitCode := managedProcessExitDisposition(exitErr)
	if intentionalStop {
		disposition = types.ManagedStartupExitIntentional
		exitCode = nil
	}
	diagnostics := npmresolution.AnalyzeManagedStartupDiagnostics(strings.Join(stderr, "\n"))
	diagnosticComplete := collectionComplete && stderrRetainedComplete && diagnostics.Complete
	npmCode := ""
	if diagnosticComplete {
		npmCode = preferredManagedStartupCode(diagnostics.Codes)
	}
	return &types.ManagedStartupEvidence{
		ProcessGeneration:     generation,
		ExitDisposition:       disposition,
		ExitCode:              exitCode,
		NPMCode:               npmCode,
		CollectionComplete:    collectionComplete,
		NPMDiagnosticPresent:  diagnostics.Present,
		NPMDiagnosticComplete: diagnosticComplete,
		UnclassifiedNPMCode:   diagnostics.UnclassifiedCode,
	}
}

func preferredManagedStartupCode(codes []string) string {
	for _, code := range codes {
		if npmresolution.IsPermanentManagedStartupCode(code) {
			return code
		}
	}
	for _, code := range codes {
		if code == "ETARGET" {
			return code
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return codes[0]
}
