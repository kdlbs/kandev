// Package intent identifies built-in authoring requests independently of prompt text.
package intent

import (
	"context"
	"errors"
)

type Intent string

const Walkthrough Intent = "walkthrough"
const MetadataKey = "builtin_intent"

var ErrInvalid = errors.New("unknown built-in tool request")
var ErrUnavailable = errors.New("the built-in tools required for this request are unavailable; refresh the session MCP details")

func (i Intent) Validate() error {
	if i != "" && i != Walkthrough {
		return ErrInvalid
	}
	return nil
}

// ValidateRequest fails closed when a typed request has no authority resolver.
func ValidateRequest(ctx context.Context, resolver any, taskID, sessionID string, value Intent) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value == "" {
		return nil
	}
	checker, ok := resolver.(interface {
		ValidateBuiltinIntent(context.Context, string, string, string) error
	})
	if !ok {
		return ErrUnavailable
	}
	return checker.ValidateBuiltinIntent(ctx, taskID, sessionID, string(value))
}

// FromMetadata rejects malformed persisted intent instead of treating it as ordinary text.
func FromMetadata(metadata map[string]interface{}) (Intent, error) {
	value, exists := metadata[MetadataKey]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", ErrInvalid
	}
	intent := Intent(text)
	return intent, intent.Validate()
}

// PrepareRequest requires a server resolver that carries dispatch authority.
func PrepareRequest(ctx context.Context, resolver any, taskID, sessionID string, value Intent) (context.Context, error) {
	if err := value.Validate(); err != nil {
		return ctx, err
	}
	if value == "" {
		return ctx, nil
	}
	preparer, ok := resolver.(interface {
		PrepareBuiltinIntent(context.Context, string, string, string) (context.Context, error)
	})
	if !ok {
		return ctx, ErrUnavailable
	}
	return preparer.PrepareBuiltinIntent(ctx, taskID, sessionID, string(value))
}
