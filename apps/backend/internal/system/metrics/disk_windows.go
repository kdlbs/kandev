//go:build windows

package metrics

import (
	"context"
	"errors"
)

func diskPercent(_ context.Context, _ string) (float64, error) {
	return 0, errors.New("disk usage unavailable on windows")
}
