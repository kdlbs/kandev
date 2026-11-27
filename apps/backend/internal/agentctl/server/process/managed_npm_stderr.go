package process

import (
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/npmresolution"
)

const (
	npmETargetCodeLine      = "npm error code ETARGET"
	npmNotargetPrefix       = "npm error notarget No matching version found for "
	npmLegacyNotargetPrefix = "npm ERR! notarget No matching version found for "
	npmUnknownCodeMarker    = "npm error code UNKNOWN"
)

func safeManagedNpmStderrLine(raw string) (string, bool) {
	line := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(line), "npm error syscall ") || strings.HasPrefix(strings.ToLower(line), "npm err! syscall ") {
		fields := strings.Fields(line)
		if len(fields) == 4 && (strings.EqualFold(fields[3], "open") || strings.EqualFold(fields[3], "read")) {
			return "npm error syscall " + strings.ToLower(fields[3]), true
		}
		return "npm error diagnostic redacted", true
	}
	if strings.HasPrefix(strings.ToLower(line), "npm error path ") || strings.HasPrefix(strings.ToLower(line), "npm err! path ") {
		return "npm error path <redacted>", true
	}
	if strings.EqualFold(line, npmETargetCodeLine) || strings.EqualFold(line, "npm ERR! code ETARGET") {
		return npmETargetCodeLine, true
	}
	startupDiagnostic := npmresolution.AnalyzeManagedStartupDiagnostics(line)
	if startupDiagnostic.Present {
		if !startupDiagnostic.Complete {
			return "npm error diagnostic incomplete", true
		}
		if startupDiagnostic.UnclassifiedCode {
			return npmUnknownCodeMarker, true
		}
		if len(startupDiagnostic.Codes) == 1 {
			return "npm error code " + startupDiagnostic.Codes[0], true
		}
	}
	if packageSpec, ok := npmresolution.RawReleaseAgePolicyPackageSpec(line); ok {
		if managedruntime.ValidateExactPackageSpec(packageSpec) != nil {
			return "", false
		}
		return npmNotargetPrefix + packageSpec + " with a date before " + npmresolution.ReleaseDateMarker, true
	}

	for _, prefix := range []string{npmNotargetPrefix, npmLegacyNotargetPrefix} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		packageSpec := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, prefix)), ".")
		if managedruntime.ValidateExactPackageSpec(packageSpec) != nil {
			return "", false
		}
		return npmNotargetPrefix + packageSpec, true
	}
	if strings.HasPrefix(strings.ToLower(line), "npm error ") || strings.HasPrefix(strings.ToLower(line), "npm err! ") {
		return "npm error diagnostic redacted", true
	}
	return "", false
}

const maxManagedNPMRawDiagnosticBytes = 16 << 10

func (m *Manager) appendManagedNPMRawDiagnostic(raw string) {
	line := strings.TrimSpace(raw)
	lower := strings.ToLower(line)
	if !strings.HasPrefix(lower, "npm error code ") && !strings.HasPrefix(lower, "npm err! code ") &&
		!strings.HasPrefix(lower, "npm error syscall ") && !strings.HasPrefix(lower, "npm err! syscall ") &&
		!strings.HasPrefix(lower, "npm error path ") && !strings.HasPrefix(lower, "npm err! path ") {
		return
	}
	m.stderrMu.Lock()
	defer m.stderrMu.Unlock()
	if m.managedNPMRawOverflow {
		return
	}
	lineBytes := len(line) + 1
	if lineBytes > maxManagedNPMRawDiagnosticBytes-m.managedNPMRawBytes || !utf8.ValidString(line) {
		m.managedNPMRawOverflow = true
		m.managedNPMRawLines = nil
		m.managedNPMRawBytes = 0
		return
	}
	m.managedNPMRawLines = append(m.managedNPMRawLines, line)
	m.managedNPMRawBytes += lineBytes
}

func (m *Manager) managedNPMRawDiagnosticSnapshot() (string, bool) {
	m.stderrMu.RLock()
	defer m.stderrMu.RUnlock()
	return strings.Join(m.managedNPMRawLines, "\n"), !m.managedNPMRawOverflow
}

func (m *Manager) clearManagedNPMRawDiagnostic() {
	m.stderrMu.Lock()
	defer m.stderrMu.Unlock()
	m.managedNPMRawLines = nil
	m.managedNPMRawBytes = 0
	m.managedNPMRawOverflow = false
}
