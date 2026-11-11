//go:build !unix

package process

import "os"

func projectMCPReadFlags() int {
	return os.O_RDONLY
}
