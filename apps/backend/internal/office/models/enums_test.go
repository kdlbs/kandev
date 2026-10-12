package models_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestBudgetPeriod_Total pins AC-OFFICE-BUDGET-002.8: `total` is a declared
// period value, not a fallthrough from an unrecognized one.
func TestBudgetPeriod_Total(t *testing.T) {
	if !models.BudgetPeriodTotal.Valid() {
		t.Errorf("BudgetPeriodTotal.Valid() = false, want true")
	}
	if models.BudgetPeriodTotal.String() != "total" {
		t.Errorf("BudgetPeriodTotal.String() = %q, want \"total\"", models.BudgetPeriodTotal.String())
	}
}

func TestBudgetPeriod_ValidSet(t *testing.T) {
	valid := []models.BudgetPeriod{
		models.BudgetPeriodDaily,
		models.BudgetPeriodMonthly,
		models.BudgetPeriodYearly,
		models.BudgetPeriodTotal,
	}
	for _, p := range valid {
		if !p.Valid() {
			t.Errorf("%q.Valid() = false, want true", p)
		}
	}
	if models.BudgetPeriod("weekly").Valid() {
		t.Error(`BudgetPeriod("weekly").Valid() = true, want false`)
	}
}
