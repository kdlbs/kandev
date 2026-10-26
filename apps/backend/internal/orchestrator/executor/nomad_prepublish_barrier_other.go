//go:build !unix

package executor

import "os"

func ownedByCurrentUser(os.FileInfo) bool { return false }
