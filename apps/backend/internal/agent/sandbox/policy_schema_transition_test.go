package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicySchemaChangeRequiresReviewedMigration(t *testing.T) {
	for _, direction := range []string{"upgrade", "downgrade"} {
		t.Run(direction, func(t *testing.T) {
			service, err := NewService(sqlitePolicyDB(t), true, nil)
			require.NoError(t, err)
			before, after := policyFixture(t), typedPolicyFixture()
			if direction == "downgrade" {
				before, after = after, before
			}
			source, err := service.Publish(operatorContext(t), before, 0)
			require.NoError(t, err)
			after.ID = source.Policy.ID
			_, err = service.Publish(operatorContext(t), after, source.Number)
			require.ErrorIs(t, err, ErrIneligible, "ordinary publication must not change policy meaning through a version switch")
			latest, err := service.GetLatest(operatorContext(t), source.Policy.ID)
			require.NoError(t, err)
			require.Equal(t, source, latest)
		})
	}
}

func TestPolicyPublicationMissingExpectedHeadIsConflict(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	_, err = service.Publish(operatorContext(t), typedPolicyFixture(), 1)
	require.ErrorIs(t, err, ErrConflict)
}
