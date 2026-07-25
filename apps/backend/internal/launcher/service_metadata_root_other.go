//go:build !unix

package launcher

import "fmt"

func openSystemNativeMetadataHome(string) (nativeMetadataRoot, error) {
	return nil, fmt.Errorf("system service metadata confinement is unsupported on this platform")
}
