package sqlite

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

var errUsageSupersessionUnavailable = errors.New("superseded item is unavailable")

func (r *Repository) previousUsageObservationTx(ctx context.Context, tx *sqlx.Tx, stored observationItem, input pluginsdk.UsageObservationV1) (*indexedObservation, string, error) {
	if stored.SourceTable == "" {
		return nil, "", nil
	}
	value, err := r.loadIndexedObservationTx(ctx, tx, stored.SourceTable, stored.ObservationID)
	if err != nil {
		return nil, "", err
	}
	if old := value.observation(); old != nil && !sameUsageSegmentDeclaration(old.Segment, input.Segment) {
		return nil, "an item cannot change its segment lineage; supersede the item explicitly", nil
	}
	return &value, "", nil
}

func (r *Repository) prepareUsageObservationTargetTx(ctx context.Context, tx *sqlx.Tx, source string, input pluginsdk.UsageObservationV1, measurement models.SessionUsageMeasurement) (retainedTarget, string, error) {
	target, err := r.resolveCollectionBindingTx(ctx, tx, source, input.SubjectBinding)
	if err != nil {
		return target, ErrUsageNotVisible.Error(), nil
	}
	if err := r.admitRetainedCorrectionTx(ctx, tx, target); err != nil {
		return target, "", err
	}
	_, _, event, err := r.resolveContractWorkIdentityTx(ctx, tx, target, source, input.WorkIdentity)
	if err != nil {
		return target, "", err
	}
	if err := r.validateUsageEventCoverageTx(ctx, tx, target, event, measurement); err != nil {
		return target, err.Error(), nil
	}
	if err := r.admitUsageSourceContextTx(ctx, tx, target, event); err != nil {
		return target, err.Error(), nil
	}
	return target, "", nil
}

func (r *Repository) retireUsageSupersededItemTx(ctx context.Context, tx *sqlx.Tx, source string, target retainedTarget, input pluginsdk.UsageObservationV1) (*indexedObservation, *models.UsageMutation, error) {
	if input.SupersedesItemID == nil {
		return nil, nil, nil
	}
	old, err := r.findRetirableItemTx(ctx, tx, source, target, *input.SupersedesItemID)
	if err != nil || old.SubjectID != target.SubjectID || old.Retired != 0 || old.ItemID == input.ItemID {
		return nil, nil, errUsageSupersessionUnavailable
	}
	lineage, err := r.loadIndexedObservationTx(ctx, tx, old.SourceTable, old.ObservationID)
	if err != nil {
		return nil, nil, err
	}
	change, err := r.retireObservationItemTx(ctx, tx, old, old.Revision, old.PayloadDigest)
	return &lineage, &change, err
}

func (r *Repository) inheritUsageSegmentTx(ctx context.Context, tx *sqlx.Tx, current *indexedObservation, lineage *indexedObservation, input pluginsdk.UsageObservationV1) error {
	if lineage == nil || input.Segment != nil || lineage.Segment == "" {
		return nil
	}
	current.Segment, current.Layout, current.Bundle = lineage.Segment, lineage.Layout, lineage.Bundle
	return r.persistUsageSegmentTx(ctx, tx, current, &input)
}
