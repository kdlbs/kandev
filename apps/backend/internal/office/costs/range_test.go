package costs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-OFFICE-COSTS-001.2
func TestCostRangeWindow(t *testing.T) {
	for _, tc := range []struct{ name, value, now, start string }{
		{"month rollover", "mtd", "2026-11-01T00:00:00Z", "2026-11-01T00:00:00Z"},
		{"year rollover", "mtd", "2027-01-01T01:00:00Z", "2027-01-01T00:00:00Z"},
		{"offset previous UTC month", "mtd", "2026-03-01T00:30:00+09:00", "2026-02-01T00:00:00Z"},
		{"leap February", "30d", "2024-03-01T12:00:00Z", "2024-01-31T12:00:00Z"},
		{"DST offset", "30d", "2026-03-30T12:00:00+01:00", "2026-02-28T11:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := parseWindowTime(t, tc.now)
			got, err := costRangeWindow(tc.value, now)
			if err != nil {
				t.Fatal(err)
			}
			if !got.start.Equal(parseWindowTime(t, tc.start)) || !got.before.Equal(now) || got.start.Location() != time.UTC || got.before.Location() != time.UTC {
				t.Errorf("window=%+v", got)
			}
		})
	}
	if window, err := costRangeWindow("", time.Now()); window != nil || err != nil {
		t.Errorf("lifetime=%v/%v", window, err)
	}
	if _, err := costRangeWindow("year", time.Now()); !errors.Is(err, ErrInvalidCostRange) {
		t.Errorf("invalid error=%v", err)
	}
}

func parseWindowTime(t *testing.T, value string) time.Time {
	t.Helper()
	got, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

type windowCaptureRepo struct {
	Repository
	mu      sync.Mutex
	windows map[string]costWindow
}

func (r *windowCaptureRepo) capture(name string, start, before time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.windows[name] = costWindow{start: start, before: before}
}
func (r *windowCaptureRepo) SumCostsInWindow(_ context.Context, _ string, start, before time.Time) (int64, error) {
	r.capture("total", start, before)
	return 0, nil
}
func (r *windowCaptureRepo) GetCostsByAgentInWindow(_ context.Context, _ string, start, before time.Time) ([]*CostBreakdown, error) {
	r.capture("agent", start, before)
	return nil, nil
}
func (r *windowCaptureRepo) GetCostsByProjectInWindow(_ context.Context, _ string, start, before time.Time) ([]*CostBreakdown, error) {
	r.capture("project", start, before)
	return nil, nil
}
func (r *windowCaptureRepo) GetCostsByModelInWindow(_ context.Context, _ string, start, before time.Time) ([]*CostBreakdown, error) {
	r.capture("model", start, before)
	return nil, nil
}
func (r *windowCaptureRepo) GetCostsByProviderInWindow(_ context.Context, _ string, start, before time.Time) ([]*CostBreakdown, error) {
	r.capture("provider", start, before)
	return nil, nil
}

// @covers AC-OFFICE-COSTS-001.2: every concurrent read receives identical instants.
func TestCostsBreakdownRangeSharedWindow(t *testing.T) {
	repo := &windowCaptureRepo{windows: map[string]costWindow{}}
	svc := NewCostService(repo, logger.Default(), nil, nil, nil)
	_, _, _, _, _, err := svc.GetCostsBreakdownForRange(context.Background(), "ws", "30d")
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.windows) != 5 {
		t.Fatalf("reads=%d, want 5", len(repo.windows))
	}
	want := repo.windows["total"]
	for name, window := range repo.windows {
		if window != want || window.before.Sub(window.start) != 720*time.Hour {
			t.Errorf("%s window=%+v, want %+v", name, window, want)
		}
	}
	clear(repo.windows)
	_, _, _, _, _, err = svc.GetCostsBreakdownForRange(context.Background(), "ws", "invalid")
	if !errors.Is(err, ErrInvalidCostRange) || len(repo.windows) != 0 {
		t.Errorf("invalid range=%v reads=%d", err, len(repo.windows))
	}
}
