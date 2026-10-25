//go:build !linux

package inventoryrepair

import (
	"context"
	"errors"
)

func checkProcesses(context.Context, Plan) error {
	return errors.New("offline inventory application requires Linux process inspection; preview is available on all platforms")
}
