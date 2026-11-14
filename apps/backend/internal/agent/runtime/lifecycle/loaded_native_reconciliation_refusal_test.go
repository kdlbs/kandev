package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithJournalUnresolvedRefusalPreservesSpecificReason(t *testing.T) {
	cause := errors.New("peer record is absent")
	classified := withDeliveryRecoveryRefusal(DeliveryRecoveryRefusalPeerRecordMissing, cause)

	got := withJournalUnresolvedRefusal(classified)
	var reasoner interface{ DeliveryRecoveryRefusalReason() string }
	require.ErrorAs(t, got, &reasoner)
	require.Equal(t, DeliveryRecoveryRefusalPeerRecordMissing, reasoner.DeliveryRecoveryRefusalReason())
	require.ErrorIs(t, got, cause)

	got = withJournalUnresolvedRefusal(errors.New("journal acknowledgement failed"))
	require.ErrorAs(t, got, &reasoner)
	require.Equal(t, DeliveryRecoveryRefusalJournalUnresolved, reasoner.DeliveryRecoveryRefusalReason())
}
