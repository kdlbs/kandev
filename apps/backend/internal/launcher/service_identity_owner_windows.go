//go:build windows

package launcher

import "fmt"

func nativePathOwnerUID(string) (int, error) {
	return 0, fmt.Errorf("system service home ownership is unsupported on windows")
}
