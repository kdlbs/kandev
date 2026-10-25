package canvas

import (
	"expvar"
	"strconv"
	"testing"
)

func canvasScopeMetricCount(t *testing.T, label string) int64 {
	t.Helper()
	value := expvar.Get("canvas_data_scope_transition_total")
	if value == nil {
		t.Fatal("canvas data-scope transition counter is not registered")
	}
	metrics, ok := value.(*expvar.Map)
	if !ok {
		t.Fatalf("canvas data-scope transition metric has type %T, want *expvar.Map", value)
	}
	var count int64
	metrics.Do(func(keyValue expvar.KeyValue) {
		if keyValue.Key != label {
			return
		}
		parsed, err := strconv.ParseInt(keyValue.Value.String(), 10, 64)
		if err != nil {
			t.Errorf("canvas data-scope metric %q value = %q: %v", label, keyValue.Value.String(), err)
			return
		}
		count = parsed
	})
	return count
}

func TestRecordCanvasScopeTransitionUsesFixedLabels(t *testing.T) {
	label := "transition=reviewed_upgrade;result=data_scope_enabled"
	before := canvasScopeMetricCount(t, label)
	recordCanvasScopeTransition(canvasScopeReviewedUpgrade, canvasScopeEnabled)
	after := canvasScopeMetricCount(t, label)
	if after-before != 1 {
		t.Fatalf("transition counter delta = %d, want 1", after-before)
	}
}
