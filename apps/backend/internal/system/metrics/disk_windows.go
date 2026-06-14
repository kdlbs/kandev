//go:build windows

package metrics

import "errors"

func diskPercent(_ string) (float64, error) {
	return 0, errors.New("disk usage unavailable on windows")
}
