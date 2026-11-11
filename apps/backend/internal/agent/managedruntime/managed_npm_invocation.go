package managedruntime

import (
	"errors"
	"strings"
)

const managedNPMInvocationHeadLength = 6

const (
	managedNPMOfflinePreference = "--prefer-offline"
	managedNPMOnlinePreference  = "--prefer-online"
)

// ManagedNPMInvocation is the validated structural boundary of one direct
// managed npx launch. Agent arguments after the required runtime arguments are
// opaque and never participate in installation authority.
type ManagedNPMInvocation struct {
	args            []string
	PackageSpec     string
	PackageIndex    int
	PreferenceIndex int
	agentArgsIndex  int
}

// ParseManagedNPMInvocation validates the canonical direct npx shape:
//
//	npx --yes <preference> --prefix <trusted-marker> <exact-package> <required...> <agent...>
//
// The package position is structural, never discovered by scanning agent
// values. Wrappers, alternative npm forms, altered npm options, and a missing
// or wrong required runtime argument are rejected.
func ParseManagedNPMInvocation(command []string, packageName string, requiredArgs []string) (ManagedNPMInvocation, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return ManagedNPMInvocation{}, errors.New("managed npm package is unavailable")
	}
	if len(command) < managedNPMInvocationHeadLength {
		return ManagedNPMInvocation{}, errors.New("managed npm command is not a direct npx invocation")
	}
	if command[0] != "npx" || command[1] != "--yes" {
		return ManagedNPMInvocation{}, errors.New("managed npm command is not a direct npx invocation")
	}
	if command[2] != managedNPMOfflinePreference && command[2] != managedNPMOnlinePreference {
		return ManagedNPMInvocation{}, errors.New("managed npm command has an unsupported preference")
	}
	if command[3] != "--prefix" || command[4] != NPMProjectPrefix {
		return ManagedNPMInvocation{}, errors.New("managed npm command has an unsupported project prefix")
	}
	packageSpec := command[5]
	if err := ValidateExactPackageSpec(packageSpec); err != nil {
		return ManagedNPMInvocation{}, errors.New("managed npm command does not select an exact package")
	}
	if !strings.HasPrefix(packageSpec, packageName+"@") {
		return ManagedNPMInvocation{}, errors.New("managed npm command does not select the registered package")
	}
	agentIndex := managedNPMInvocationHeadLength + len(requiredArgs)
	if len(command) < agentIndex {
		return ManagedNPMInvocation{}, errors.New("managed npm command omits required runtime arguments")
	}
	for index, required := range requiredArgs {
		if command[managedNPMInvocationHeadLength+index] != required {
			return ManagedNPMInvocation{}, errors.New("managed npm command has a different required runtime argument")
		}
	}
	return ManagedNPMInvocation{
		args:            append([]string(nil), command...),
		PackageSpec:     packageSpec,
		PackageIndex:    managedNPMInvocationHeadLength - 1,
		PreferenceIndex: 2,
		agentArgsIndex:  agentIndex,
	}, nil
}

// AgentArgs returns the authorized agent arguments after the registered
// runtime boundary. The returned slice is a copy.
func (i ManagedNPMInvocation) AgentArgs() []string {
	if i.agentArgsIndex <= 0 || i.agentArgsIndex > len(i.args) {
		return nil
	}
	return append([]string(nil), i.args[i.agentArgsIndex:]...)
}

// WithNpmPreference copies the invocation with only npm's metadata freshness
// flag changed. Agent arguments and their order are preserved exactly.
func (i ManagedNPMInvocation) WithNpmPreference(preferOnline bool) []string {
	args := append([]string(nil), i.args...)
	if len(args) <= i.PreferenceIndex {
		return args
	}
	if preferOnline {
		args[i.PreferenceIndex] = managedNPMOnlinePreference
	} else {
		args[i.PreferenceIndex] = managedNPMOfflinePreference
	}
	return args
}

// PackageNameFromExactSpec returns the package half of one validated
// name@version spec.
func PackageNameFromExactSpec(packageSpec string) (string, bool) {
	if ValidateExactPackageSpec(packageSpec) != nil {
		return "", false
	}
	separator := strings.LastIndexByte(packageSpec, '@')
	if separator <= 0 {
		return "", false
	}
	return packageSpec[:separator], true
}
