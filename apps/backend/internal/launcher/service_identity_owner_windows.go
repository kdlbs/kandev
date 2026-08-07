//go:build windows

package launcher

import (
	"fmt"
	"os"
)

func nativeFileOwnerUID(os.FileInfo) (int, error) {
	return 0, fmt.Errorf("system service home ownership is unsupported on windows")
}
