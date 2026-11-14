package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReconcileLoadedNativeDeliveryMissingExecutionReason(t *testing.T) {
	manager := newTestManager(t)
	_, err := manager.ReconcileLoadedNativeDelivery(context.Background(), "missing-session")
	require.Error(t, err)
	reasoner, ok := err.(interface{ DeliveryRecoveryRefusalReason() string })
	require.True(t, ok)
	require.Equal(t, DeliveryRecoveryRefusalExecutionUnavailable, reasoner.DeliveryRecoveryRefusalReason())
}

func TestValidateNativeResumeSQLHistoryUnavailableReason(t *testing.T) {
	manager := newTestManager(t)
	err := manager.validateNativeResumeSQLHistory(context.Background(), nil, nil, nil, false)
	require.Error(t, err)
	reasoner, ok := err.(interface{ DeliveryRecoveryRefusalReason() string })
	require.True(t, ok)
	require.Equal(t, DeliveryRecoveryRefusalEvidenceUnavailable, reasoner.DeliveryRecoveryRefusalReason())
}
