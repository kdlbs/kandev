package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyProviderSchemaNegotiation(t *testing.T) {
	legacy := &Revision{SchemaVersion: 1}
	typed := &Revision{SchemaVersion: 2, Policy: typedPolicyFixture()}
	for _, test := range []struct {
		name     string
		revision *Revision
		major    uint32
		schemas  []uint32
		want     bool
	}{
		{"legacy implicit schema", legacy, 1, nil, true},
		{"legacy cannot accept typed", typed, 1, []uint32{2}, false},
		{"major 2 explicit legacy", legacy, 2, []uint32{1}, true},
		{"major 2 explicit typed", typed, 2, []uint32{2}, true},
		{"major 2 without schemas", typed, 2, nil, false},
		{"major 2 wrong schema", typed, 2, []uint32{1}, false},
		{"major 2 unsupported legacy", legacy, 2, []uint32{2}, false},
		{"unknown protocol", typed, 3, []uint32{2}, false},
		{"unknown revision schema", &Revision{SchemaVersion: 99}, 2, []uint32{99}, false},
		{"absent revision", nil, 1, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.revision.SupportedBy(test.major, test.schemas))
		})
	}
}
