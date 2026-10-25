// Package origin carries trusted, process-local MCP transport attestations.
// Wire payloads cannot create these markers.
package origin

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type externalTransportKey struct{}
type internalCallKey struct{}

const trustedInternalCallMetadataKey = "kandev_internal_call"

// WithTrustedInternalCall marks a server-originated dispatch that is not
// reachable from an MCP client payload.
func WithTrustedInternalCall(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalCallKey{}, true)
}

func IsTrustedInternalCall(ctx context.Context) bool {
	trusted, _ := ctx.Value(internalCallKey{}).(bool)
	return trusted
}

// AttachTrustedInternalCall carries a server-created attestation over the
// agentctl bridge. Only ChannelBackendClient calls this for its own internal
// follow-up requests; inbound MCP tool payloads cannot create it.
func AttachTrustedInternalCall(msg *ws.Message) {
	msg.EnsureMetadata()[trustedInternalCallMetadataKey] = "1"
}

// MessageCarriesTrustedInternalCall reports whether a bridge request carries
// the server-created internal attestation.
func MessageCarriesTrustedInternalCall(msg *ws.Message) bool {
	return msg != nil && msg.Metadata[trustedInternalCallMetadataKey] == "1"
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
