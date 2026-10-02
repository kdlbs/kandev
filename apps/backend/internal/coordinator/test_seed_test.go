package coordinator

import (
	"context"
	"testing"
)

func seedEligibility(t *testing.T, req SeedHistoryRequest) EligibilityResult {
	t.Helper()
	_, c, _, svc, _ := automaticFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	if err := svc.SeedCreateTaskHistory(context.Background(), c.ID, req); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSeedCreateTaskHistory_EarnsEveryLogCondition(t *testing.T) {
	res := seedEligibility(t, SeedHistoryRequest{WindowRows: 20, OldestDaysAgo: 31})
	for _, name := range []string{conditionHistory30d, conditionVolume, conditionUnedited, conditionNoUndo} {
		if !conditionMet(t, res, name) {
			t.Fatalf("%s not met: %+v", name, res.Conditions)
		}
	}
}

func TestSeedCreateTaskHistory_EditedRowsFailTheRate(t *testing.T) {
	res := seedEligibility(t, SeedHistoryRequest{WindowRows: 20, EditedRows: 3, OldestDaysAgo: 31})
	if conditionMet(t, res, conditionUnedited) {
		t.Fatalf("85%% counted as met: %+v", res.Conditions)
	}
}

func TestSeedCreateTaskHistory_WithoutOldRowFailsHistory(t *testing.T) {
	res := seedEligibility(t, SeedHistoryRequest{WindowRows: 20})
	if conditionMet(t, res, conditionHistory30d) {
		t.Fatalf("history met without an old row: %+v", res.Conditions)
	}
}

func TestSeedHistoryRequest_Validate(t *testing.T) {
	for name, req := range map[string]SeedHistoryRequest{
		"negative rows":    {WindowRows: -1},
		"too many rows":    {WindowRows: maxSeedRows + 1},
		"edited over rows": {WindowRows: 2, EditedRows: 3},
		"absurd age":       {OldestDaysAgo: 400},
	} {
		if req.validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
