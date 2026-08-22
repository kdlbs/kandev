package lifecycle

import (
	"errors"
	"strings"
)

// ErrModelIdentityBlock identifies a model value that cannot be bound to a
// concrete provider/model identity. Callers must fail the launch rather than
// substituting an empty, provider-default, or fallback model.
var ErrModelIdentityBlock = errors.New("BLOCKED_MODEL_IDENTITY")

// ValidateModelIdentity rejects values that are deferred configuration rather
// than concrete agent/model identifiers. An empty model is valid only as an
// explicit provider-default selection; an empty provider is never valid.
func ValidateModelIdentity(provider, model string) error {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || hasOpaqueModelIdentityShape(provider) || hasOpaqueModelIdentityShape(model) {
		return ErrModelIdentityBlock
	}
	return nil
}

func hasOpaqueModelIdentityShape(value string) bool {
	if value == "" {
		return false
	}
	if strings.ContainsAny(value, "=\r\n") {
		return true
	}
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return true
	}
	if strings.HasPrefix(value, "{{") && strings.HasSuffix(value, "}}") {
		return true
	}
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return true
	}
	if strings.HasPrefix(value, "__") && strings.HasSuffix(value, "__") && len(value) > 4 {
		return true
	}
	return strings.HasPrefix(value, "$") && len(value) > 1
}
