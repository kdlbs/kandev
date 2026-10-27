package sandbox

import (
	"slices"
	"strings"
)

type OverrideConstraints struct {
	ForbiddenArguments   []string `json:"forbidden_arguments"`
	ForbiddenEnvironment []string `json:"forbidden_environment"`
}

// ValidateOverrides checks untrusted profile overrides before provider compilation.
func ValidateOverrides(arguments []string, environment map[string]string, constraints OverrideConstraints) error {
	for _, argument := range arguments {
		flag, _, _ := strings.Cut(argument, "=")
		if slices.Contains(constraints.ForbiddenArguments, flag) {
			return ErrInvalidPolicy
		}
	}
	for key := range environment {
		if privateEnvironmentKey(key) || slices.Contains(constraints.ForbiddenEnvironment, key) {
			return ErrInvalidPolicy
		}
	}
	return nil
}
