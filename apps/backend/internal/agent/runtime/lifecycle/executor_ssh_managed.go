package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
	"strconv"
)

const (
	MetadataKeySSHConnectionID              = "ssh_connection_id"
	MetadataKeySSHRouteStamp                = "ssh_route_stamp"
	MetadataKeySSHIdentityGeneration        = "ssh_identity_generation"
	MetadataKeySSHBastionIdentityGeneration = "ssh_bastion_identity_generation"
)

type ManagedSSHConnections interface {
	AdmitSavedRuntime(context.Context, string) (sshconnections.RuntimeSnapshot, error)
	AdmitRuntime(context.Context, string, string, string) (sshconnections.RuntimeSnapshot, error)
	DialRuntime(context.Context, sshconnections.RuntimeSnapshot) (*ssh.Client, error)
}

func (r *SSHExecutor) SetManagedConnections(resolver ManagedSSHConnections) {
	r.managedConnections = resolver
}

func (r *SSHExecutor) dialSession(ctx context.Context, sessionID string, md map[string]interface{}, allowAdmission bool) (*SSHTarget, *ssh.Client, error) {
	id := getMetadataString(md, MetadataKeySSHConnectionID)
	if !allowAdmission && id == "" {
		return r.dialRetainedLegacySession(ctx, sessionID, md)
	}
	if r.managedConnections == nil {
		if id == "" {
			return nil, nil, sshconnections.ErrInvalidRoute
		}
		return nil, nil, sshconnections.ErrDisabled
	}
	stamp := getMetadataString(md, MetadataKeySSHRouteStamp)
	var snapshot sshconnections.RuntimeSnapshot
	var err error
	if allowAdmission {
		stamp = ""
		snapshot, err = r.managedConnections.AdmitSavedRuntime(ctx, sessionID)
	} else {
		if stamp == "" {
			return nil, nil, sshconnections.ErrConflict
		}
		snapshot, err = r.managedConnections.AdmitRuntime(ctx, sessionID, id, stamp)
	}
	if err != nil {
		return nil, nil, err
	}
	if stamp != "" && (getMetadataString(md, MetadataKeySSHIdentityGeneration) != snapshot.IdentityGeneration || getMetadataString(md, MetadataKeySSHBastionIdentityGeneration) != snapshot.BastionIdentityGeneration) {
		return nil, nil, sshconnections.ErrConflict
	}
	md[MetadataKeySSHConnectionID] = snapshot.Destination.ID
	md[MetadataKeySSHRouteStamp] = snapshot.Stamp
	md[MetadataKeySSHIdentityGeneration] = snapshot.IdentityGeneration
	md[MetadataKeySSHBastionIdentityGeneration] = snapshot.BastionIdentityGeneration
	c := snapshot.Destination
	target := &SSHTarget{Host: c.Host, Port: c.Port, User: c.User, PinnedFingerprint: c.Fingerprint}
	client, err := r.managedConnections.DialRuntime(ctx, snapshot)
	if err != nil && stamp == "" {
		delete(md, MetadataKeySSHRouteStamp)
		delete(md, MetadataKeySSHIdentityGeneration)
		delete(md, MetadataKeySSHBastionIdentityGeneration)
	}
	return target, client, err
}

type retainedLegacySSHConnections interface {
	DialRetainedLegacySession(context.Context, string, sshtransport.SSHTarget) (*ssh.Client, error)
}

func (r *SSHExecutor) dialRetainedLegacySession(ctx context.Context, sessionID string, md map[string]interface{}) (*SSHTarget, *ssh.Client, error) {
	resolver, ok := r.managedConnections.(retainedLegacySSHConnections)
	if !ok {
		return nil, nil, sshconnections.ErrDisabled
	}
	if getMetadataString(md, MetadataKeySSHHostAlias) != "" {
		return nil, nil, sshconnections.ErrInvalidRoute
	}
	port, _ := strconv.Atoi(getMetadataString(md, MetadataKeySSHPort))
	target := &SSHTarget{
		Host: getMetadataString(md, MetadataKeySSHHost), Port: port, User: getMetadataString(md, MetadataKeySSHUser),
		PinnedFingerprint: getMetadataString(md, MetadataKeySSHHostFingerprint),
		IdentitySource:    SSHIdentitySource(getMetadataString(md, MetadataKeySSHIdentitySource)),
		IdentityFile:      getMetadataString(md, MetadataKeySSHIdentityFile),
		IdentityAgent:     getMetadataString(md, MetadataKeySSHIdentityAgent),
		ProxyJump:         getMetadataString(md, MetadataKeySSHProxyJump),
	}
	client, err := resolver.DialRetainedLegacySession(ctx, sessionID, *target)
	return target, client, err
}

func copyManagedSSHMetadata(dst, src map[string]interface{}) {
	for _, key := range []string{MetadataKeySSHConnectionID, MetadataKeySSHRouteStamp, MetadataKeySSHIdentityGeneration, MetadataKeySSHBastionIdentityGeneration} {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}

// IsSSHRuntimeRouteMetadataKey identifies the transport evidence carried by
// the retained record when a controller is being reattached.
func IsSSHRuntimeRouteMetadataKey(key string) bool {
	switch key {
	case MetadataKeySSHConnectionID, MetadataKeySSHRouteStamp, MetadataKeySSHIdentityGeneration, MetadataKeySSHBastionIdentityGeneration,
		MetadataKeySSHHost, MetadataKeySSHHostAlias, MetadataKeySSHPort, MetadataKeySSHUser, MetadataKeySSHHostFingerprint,
		MetadataKeySSHIdentitySource, MetadataKeySSHIdentityFile, MetadataKeySSHProxyJump, MetadataKeySSHIdentityAgent, "ssh_connection_stamp":
		return true
	default:
		return false
	}
}
