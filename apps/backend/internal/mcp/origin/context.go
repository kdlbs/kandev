// Package origin carries trusted, process-local MCP transport attestations.
// Wire payloads cannot create these markers.
package origin

import "context"

type externalTransportKey struct{}
type managedTransportKey struct{}

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

// WithTrustedManagedTransport marks a callback request only after its
// operation-scoped grant and local task authority have been validated.
func WithTrustedManagedTransport(ctx context.Context) context.Context {
	return context.WithValue(ctx, managedTransportKey{}, true)
}

// IsTrustedManagedTransport reports whether the request entered through the
// managed-runtime callback boundary.
func IsTrustedManagedTransport(ctx context.Context) bool {
	trusted, _ := ctx.Value(managedTransportKey{}).(bool)
	return trusted
}
