// Package origin carries trusted, process-local MCP transport attestations.
// Wire payloads cannot create these markers.
package origin

import "context"

type externalTransportKey struct{}

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
