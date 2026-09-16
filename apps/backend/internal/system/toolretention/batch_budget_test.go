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
	} {
		if got := rowFitsBatch(tt.used, tt.size); got != tt.want {
			t.Errorf("used=%d next=%d: got %v want %v", tt.used, tt.size, got, tt.want)
		}
	}
}
