//go:build windows

package launcher

import (
	"fmt"
	"os"
)

var (
	lookupNativeServiceOwner = func(string) (int, int, error) {
		return 0, 0, fmt.Errorf("system service metadata ownership is unsupported on windows")
	}
	chownNativeServiceMetadata = func(*os.File, int, int) error {
		return fmt.Errorf("system service metadata ownership is unsupported on windows")
	}
)
