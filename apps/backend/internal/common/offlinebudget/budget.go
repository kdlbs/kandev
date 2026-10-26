// Package offlinebudget parses the per-executor-profile offline budget, the
// longest an agent instance may run without a stable backend connection.
package offlinebudget

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// Key is the executor profile key and instance creation field.
	Key = "offline_budget_minutes"
	// DefaultMinutes applies when no value is supplied.
	DefaultMinutes = 15
	MinMinutes     = 1
	MaxMinutes     = 1440
)

// ErrInvalid is returned for a value outside the accepted form or range.
var ErrInvalid = errors.New("invalid offline budget")

func invalid() error {
	return fmt.Errorf("%w: %s must be a whole number of minutes from %d to %d", ErrInvalid, Key, MinMinutes, MaxMinutes)
}

// Duration converts minutes to a time.Duration.
func Duration(minutes int) time.Duration { return time.Duration(minutes) * time.Minute }

// Parse reads a profile value: a base-10 integer from 1 to 1440 with
// surrounding spaces ignored. Empty means the default; a sign, fraction, unit,
// zero or overflow is invalid.
func Parse(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultMinutes, nil
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, invalid()
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < MinMinutes || n > MaxMinutes {
		return 0, invalid()
	}
	return n, nil
}

// ParseJSON reads the instance creation field. Absent or null means the
// default; anything but a JSON integer from 1 to 1440 is invalid.
func ParseJSON(raw json.RawMessage) (int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return DefaultMinutes, nil
	}
	for _, b := range raw {
		if b < '0' || b > '9' {
			return 0, invalid()
		}
	}
	return Parse(string(raw))
}
