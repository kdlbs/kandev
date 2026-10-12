package sqlite

import "testing"

func TestPostgresLifecycleReceiptExactMoveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptExactMoveRollsBackOnReceiptFailure(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-exact")
}

func TestPostgresLifecycleReceiptExactMoveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptExactMoveSuccessAndReplayAdvanceOnce(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-exact-success")
}

func TestPostgresLifecycleReceiptPromotionRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptPromotionRollsBackOnReceiptFailure(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-promotion")
}

func TestPostgresLifecycleReceiptPromotionSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptPromotionSuccessAndReplayAdvanceOnce(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-promotion-success")
}
