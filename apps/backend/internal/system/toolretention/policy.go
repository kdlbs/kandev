package toolretention

import (
	"errors"
	"time"
)

type Age struct {
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

type Policy struct {
	Enabled  bool  `json:"enabled"`
	Age      Age   `json:"age"`
	Revision int64 `json:"revision"`
}

func DefaultPolicy() Policy { return Policy{Age: Age{Value: 3, Unit: "months"}} }
func (a Age) Validate() error {
	limit := 0
	switch a.Unit {
	case "weeks":
		limit = 520
	case "months":
		limit = 120
	}
	if a.Value < 1 || a.Value > limit {
		return errors.New("invalid_age")
	}
	return nil
}
func (a Age) Cutoff(now time.Time) time.Time {
	now = now.UTC()
	if a.Unit == "weeks" {
		return now.AddDate(0, 0, -7*a.Value)
	}
	month := time.Date(now.Year(), now.Month(), 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC).AddDate(0, -a.Value, 0)
	day := min(now.Day(), month.AddDate(0, 1, -1).Day())
	return month.AddDate(0, 0, day-1)
}
