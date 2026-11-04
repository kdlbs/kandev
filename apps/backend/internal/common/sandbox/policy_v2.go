package sandbox

import (
	"net/netip"
	"strings"
)

const PolicySchemaV2 = 2

type TCPDestination struct {
	Host      string `json:"host"`
	Port      uint16 `json:"port"`
	Transport string `json:"transport"`
}

type PermissionsV2 struct {
	Filesystem      []FilesystemGrant `json:"filesystem"`
	TCP             []TCPDestination  `json:"tcp"`
	EnvironmentKeys []string          `json:"environment_keys,omitempty"`
	KandevMCP       bool              `json:"kandev_mcp"`
}

type PolicyV2 struct {
	SchemaVersion    int           `json:"schema_version"`
	StaticGrants     PermissionsV2 `json:"static_grants"`
	ApprovalEnvelope PermissionsV2 `json:"approval_envelope"`
}

func CanonicalizePolicyV2(policy PolicyV2, protectedRoots []string) (PolicyV2, error) {
	if policy.SchemaVersion != PolicySchemaV2 || policy.ApprovalEnvelope.KandevMCP || len(policy.ApprovalEnvelope.EnvironmentKeys) != 0 {
		return PolicyV2{}, ErrInvalidPolicy
	}
	static, err := canonicalPermissionsV2(policy.StaticGrants, protectedRoots)
	if err != nil {
		return PolicyV2{}, err
	}
	envelope, err := canonicalPermissionsV2(policy.ApprovalEnvelope, protectedRoots)
	if err != nil {
		return PolicyV2{}, err
	}
	return PolicyV2{SchemaVersion: PolicySchemaV2, StaticGrants: static, ApprovalEnvelope: envelope}, nil
}

// WithinCeilingV2 checks both halves of a canonical policy independently.
func WithinCeilingV2(policy PolicyV2, ceiling PermissionsV2) error {
	limit, err := permissionGrantsV2(ceiling)
	if err != nil {
		return err
	}
	return WithinLegacyCeilingV2(policy, limit)
}

// WithinLegacyCeilingV2 compares explicit eligibility with a legacy limit. It
// never creates an envelope from that limit or changes legacy route protocols.
func WithinLegacyCeilingV2(policy PolicyV2, ceiling Grants) error {
	if policy.SchemaVersion != PolicySchemaV2 {
		return ErrInvalidPolicy
	}
	for _, permissions := range []PermissionsV2{policy.StaticGrants, policy.ApprovalEnvelope} {
		grants, err := permissionGrantsV2(permissions)
		if err != nil {
			return err
		}
		if err := WithinCeiling(grants, ceiling); err != nil {
			return err
		}
	}
	return nil
}

// LegacyWithinCeilingV2 retains legacy permission meaning against a typed limit.
func LegacyWithinCeilingV2(grants Grants, ceiling PermissionsV2) error {
	limit, err := permissionGrantsV2(ceiling)
	if err != nil {
		return err
	}
	return WithinCeiling(grants, limit)
}

// WithinPolicyV2 compares static and conditional permissions independently.
func WithinPolicyV2(candidate, limit PolicyV2) error {
	if candidate.SchemaVersion != PolicySchemaV2 || limit.SchemaVersion != PolicySchemaV2 {
		return ErrInvalidPolicy
	}
	for _, pair := range [][2]PermissionsV2{
		{candidate.StaticGrants, limit.StaticGrants},
		{candidate.ApprovalEnvelope, limit.ApprovalEnvelope},
	} {
		grants, err := permissionGrantsV2(pair[0])
		if err != nil {
			return err
		}
		if err = LegacyWithinCeilingV2(grants, pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func canonicalPermissionsV2(permissions PermissionsV2, protectedRoots []string) (PermissionsV2, error) {
	grants, err := permissionGrantsV2(permissions)
	if err != nil {
		return PermissionsV2{}, err
	}
	canonical, err := Canonicalize(grants, protectedRoots)
	if err != nil {
		return PermissionsV2{}, err
	}
	destinations := make([]TCPDestination, 0, len(canonical.Network))
	for _, route := range canonical.Network {
		destinations = append(destinations, TCPDestination{Host: route.Host, Port: route.Port, Transport: route.Protocol})
	}
	return PermissionsV2{Filesystem: canonical.Filesystem, TCP: destinations, EnvironmentKeys: canonical.EnvironmentKeys, KandevMCP: canonical.KandevMCP}, nil
}

func permissionGrantsV2(permissions PermissionsV2) (Grants, error) {
	if len(permissions.TCP) > 256 {
		return Grants{}, ErrInvalidPolicy
	}
	routes := make([]NetworkGrant, 0, len(permissions.TCP))
	for _, destination := range permissions.TCP {
		if destination.Transport != ProtocolTCP {
			return Grants{}, ErrInvalidPolicy
		}
		host := strings.ToLower(destination.Host)
		if address, err := netip.ParseAddr(host); err == nil {
			host = address.Unmap().String()
		}
		route := NetworkGrant{Host: host, Port: destination.Port, Protocol: destination.Transport}
		if !validRoute(route) {
			return Grants{}, ErrInvalidPolicy
		}
		routes = append(routes, route)
	}
	return Grants{Filesystem: permissions.Filesystem, Network: routes, EnvironmentKeys: permissions.EnvironmentKeys, KandevMCP: permissions.KandevMCP}, nil
}
