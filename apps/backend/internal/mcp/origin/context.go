// Package origin carries trusted, process-local MCP transport attestations.
// Wire payloads cannot create these markers.
package origin

import "context"

type externalTransportKey struct{}
type internalCallKey struct{}

// WithTrustedInternalCall marks a server-originated dispatch that is not
// reachable from an MCP client payload.
func WithTrustedInternalCall(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalCallKey{}, true)
}

func IsTrustedInternalCall(ctx context.Context) bool {
	trusted, _ := ctx.Value(internalCallKey{}).(bool)
	return trusted
}

// WithTrustedExternalTransport marks a context after it enters through the
// backend's external MCP transport boundary.
func WithTrustedExternalTransport(ctx context.Context) context.Context {
	return context.WithValue(ctx, externalTransportKey{}, true)
}

// IsTrustedExternalTransport reports whether the external MCP boundary marked
// the context in-process.
func IsTrustedExternalTransport(ctx context.Context) bool {
	trusted, _ := ctx.Value(externalTransportKey{}).(bool)
	return trusted
}
