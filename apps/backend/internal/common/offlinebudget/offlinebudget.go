// Package offlinebudget resolves an executor profile's offline_budget_minutes
// value. It is a leaf package so profile validation and launch validation
// share one parser without importing each other.
package offlinebudget

import (
	"errors"
	"strconv"
	"strings"
)

const (
	// DefaultMinutes is used when offline_budget_minutes is absent or blank.
	DefaultMinutes = 15
	MinMinutes     = 1
	MaxMinutes     = 1440
)

// ErrInvalid is returned when offline_budget_minutes is present but is not a
// base-10 integer from 1 to 1440.
var ErrInvalid = errors.New("offline_budget_minutes must be a base-10 integer from 1 to 1440")

// Resolve returns the budget in minutes. A blank value resolves to
// DefaultMinutes; any other value must be digits only within the range.
func Resolve(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultMinutes, nil
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrInvalid
		}
	}
	minutes, err := strconv.Atoi(trimmed)
	if err != nil || minutes < MinMinutes || minutes > MaxMinutes {
		return 0, ErrInvalid
	}
	return minutes, nil
}
