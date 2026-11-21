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

func TestPostgresLifecycleReceiptCascadeArchiveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptCascadeArchiveRollsBackOnReceiptFailure(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-cascade-archive")
}

func TestPostgresLifecycleReceiptCascadeArchiveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptCascadeArchiveSuccessAndReplayAdvanceOnce(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-cascade-archive-success")
}

func TestPostgresLifecycleReceiptCascadeUnarchiveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptCascadeUnarchiveRollsBackOnReceiptFailure(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-cascade-unarchive")
}

func TestPostgresLifecycleReceiptCascadeUnarchiveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptCascadeUnarchiveSuccessAndReplayAdvanceOnce(t, newPostgresLifecycleReceiptMatrixRepo(t), "postgres-cascade-unarchive-success")
}
