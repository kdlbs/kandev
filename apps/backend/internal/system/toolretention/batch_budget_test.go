package toolretention

import "testing"

func TestBatchByteBudgetAdmitsLargeRowsOnlyAlone(t *testing.T) {
	for _, tt := range []struct {
		used, size int64
		want       bool
	}{
		{0, 32 * 1024 * 1024, true},
		{batchBytes - 1, 32 * 1024 * 1024, false},
		{batchBytes - 1, 1, true},
		{batchBytes, 1, false},
		{1, batchBytes, false},
		// O3 archived-data retention shares this predicate: a first row the size
		// of the whole budget is admitted alone; a second row is then blocked.
		{0, batchBytes, true},
		{batchBytes - 1, batchBytes, false},
		{batchBytes, batchBytes, false},
	} {
		if got := rowFitsBatch(tt.used, tt.size); got != tt.want {
			t.Errorf("used=%d next=%d: got %v want %v", tt.used, tt.size, got, tt.want)
		}
	}
}
