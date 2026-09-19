package process

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/managedruntime"
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
