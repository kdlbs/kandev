package process

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/npmresolution"
)

const (
	npmETargetCodeLine      = "npm error code ETARGET"
	npmNotargetPrefix       = "npm error notarget No matching version found for "
	npmLegacyNotargetPrefix = "npm ERR! notarget No matching version found for "
)

func safeManagedNpmStderrLine(raw string) (string, bool) {
	line := strings.TrimSpace(raw)
	if strings.EqualFold(line, npmETargetCodeLine) || strings.EqualFold(line, "npm ERR! code ETARGET") {
		return npmETargetCodeLine, true
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
	return "", false
}
