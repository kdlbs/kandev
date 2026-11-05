package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const policyTestIPv4 = "127.0.0.1"

func TestTypedPolicyCanonicalizationPreservesEmptyEnvelope(t *testing.T) {
	policy := PolicyV2{SchemaVersion: PolicySchemaV2, StaticGrants: PermissionsV2{
		Filesystem:      []FilesystemGrant{{Path: t.TempDir()}},
		EnvironmentKeys: []string{"API_TOKEN"},
		KandevMCP:       true,
		TCP: []TCPDestination{
			{Host: "API.EXAMPLE.TEST", Port: 443, Transport: "tcp"},
			{Host: "api.example.test", Port: 443, Transport: "tcp"},
			{Host: "0:0:0:0:0:0:0:1", Port: 8443, Transport: "tcp"},
			{Host: "::1", Port: 8443, Transport: "tcp"},
		},
	}}
	canonical, err := CanonicalizePolicyV2(policy, nil)
	require.NoError(t, err)
	require.Equal(t, []TCPDestination{{Host: "::1", Port: 8443, Transport: "tcp"}, {Host: "api.example.test", Port: 443, Transport: "tcp"}}, canonical.StaticGrants.TCP)
	require.Empty(t, canonical.ApprovalEnvelope.Filesystem)
	require.Empty(t, canonical.ApprovalEnvelope.TCP)
	require.Empty(t, canonical.ApprovalEnvelope.EnvironmentKeys)
	require.False(t, canonical.ApprovalEnvelope.KandevMCP)
	require.Equal(t, "API.EXAMPLE.TEST", policy.StaticGrants.TCP[0].Host, "normalization must not change the caller's captured policy")
}

func TestTypedPolicyChecksStaticAndEnvelopeAgainstCeiling(t *testing.T) {
	allowed := TCPDestination{Host: "api.example.test", Port: 443, Transport: "tcp"}
	otherPort := TCPDestination{Host: "api.example.test", Port: 8443, Transport: "tcp"}
	ceiling := PermissionsV2{TCP: []TCPDestination{allowed}}
	for _, kind := range []string{"static", "envelope"} {
		t.Run(kind, func(t *testing.T) {
			policy := PolicyV2{SchemaVersion: PolicySchemaV2}
			permissions := &policy.StaticGrants
			if kind == "envelope" {
				permissions = &policy.ApprovalEnvelope
			}
			permissions.TCP = []TCPDestination{allowed}
			require.NoError(t, WithinCeilingV2(policy, ceiling))
			permissions.TCP = []TCPDestination{otherPort}
			require.ErrorIs(t, WithinCeilingV2(policy, ceiling), ErrOutsideCeiling)
		})
	}
}

func TestTypedPolicyRejectsUnversionedAndInvalidDestinations(t *testing.T) {
	for _, version := range []int{-1, 0, 1, 3} {
		_, err := CanonicalizePolicyV2(PolicyV2{SchemaVersion: version}, nil)
		require.ErrorIs(t, err, ErrInvalidPolicy)
	}
	for _, route := range []TCPDestination{
		{Host: "api.example.test", Port: 443, Transport: "https"},
		{Host: "*.example.test", Port: 443, Transport: "tcp"},
		{Host: policyTestIPv4, Transport: "tcp"},
		{Host: "0.0.0.0", Port: 443, Transport: "tcp"},
		{Host: "::ffff:0.0.0.0", Port: 443, Transport: "tcp"},
		{Host: "::ffff:224.0.0.1", Port: 443, Transport: "tcp"},
		{Host: "fe80::1%eth0", Port: 443, Transport: "tcp"},
	} {
		for _, envelope := range []bool{false, true} {
			policy := PolicyV2{SchemaVersion: PolicySchemaV2}
			if envelope {
				policy.ApprovalEnvelope.TCP = []TCPDestination{route}
			} else {
				policy.StaticGrants.TCP = []TCPDestination{route}
			}
			_, err := CanonicalizePolicyV2(policy, nil)
			require.ErrorIs(t, err, ErrInvalidPolicy, "%+v envelope=%t", route, envelope)
		}
	}
}

func TestTypedPolicyCeilingPreservesPermissionClasses(t *testing.T) {
	root := t.TempDir()
	ceiling := PermissionsV2{Filesystem: []FilesystemGrant{{Path: root}}, EnvironmentKeys: []string{"API_TOKEN"}}
	for _, test := range []struct {
		name        string
		permissions PermissionsV2
	}{
		{"filesystem write", PermissionsV2{Filesystem: []FilesystemGrant{{Path: root, Writable: true}}}},
		{"environment key", PermissionsV2{EnvironmentKeys: []string{"OTHER_TOKEN"}}},
		{"logical MCP", PermissionsV2{KandevMCP: true}},
	} {
		for _, envelope := range []bool{false, true} {
			policy := PolicyV2{SchemaVersion: PolicySchemaV2}
			if envelope {
				policy.ApprovalEnvelope = test.permissions
			} else {
				policy.StaticGrants = test.permissions
			}
			require.ErrorIs(t, WithinCeilingV2(policy, ceiling), ErrOutsideCeiling, "%s envelope=%t", test.name, envelope)
		}
	}
}

func TestTypedPolicyProtectsFilesystemInBothHalves(t *testing.T) {
	protected := t.TempDir()
	for _, envelope := range []bool{false, true} {
		policy := PolicyV2{SchemaVersion: PolicySchemaV2}
		permissions := PermissionsV2{Filesystem: []FilesystemGrant{{Path: protected}}}
		if envelope {
			policy.ApprovalEnvelope = permissions
		} else {
			policy.StaticGrants = permissions
		}
		_, err := CanonicalizePolicyV2(policy, []string{protected})
		require.ErrorIs(t, err, ErrInvalidPolicy, "envelope=%t", envelope)
	}
}

func TestTypedPolicyCanonicalizesMappedIPv4WithoutDNSAliases(t *testing.T) {
	policy := PolicyV2{SchemaVersion: PolicySchemaV2, StaticGrants: PermissionsV2{TCP: []TCPDestination{
		{Host: "::ffff:127.0.0.1", Port: 443, Transport: "tcp"},
		{Host: policyTestIPv4, Port: 443, Transport: "tcp"},
		{Host: "localhost", Port: 443, Transport: "tcp"},
	}}}
	canonical, err := CanonicalizePolicyV2(policy, nil)
	require.NoError(t, err)
	require.Equal(t, []TCPDestination{{Host: policyTestIPv4, Port: 443, Transport: "tcp"}, {Host: "localhost", Port: 443, Transport: "tcp"}}, canonical.StaticGrants.TCP)
}

func TestTypedPolicyEnvelopeRejectsNonOperationPermissions(t *testing.T) {
	for _, envelope := range []PermissionsV2{{KandevMCP: true}, {EnvironmentKeys: []string{"API_TOKEN"}}} {
		_, err := CanonicalizePolicyV2(PolicyV2{SchemaVersion: PolicySchemaV2, ApprovalEnvelope: envelope}, nil)
		require.ErrorIs(t, err, ErrInvalidPolicy, "logical MCP and environment injection are not approval operations")
	}
}
