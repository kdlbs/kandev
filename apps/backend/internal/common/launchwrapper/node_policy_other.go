//go:build !unix

package launchwrapper

import "os"

func nodePolicyOwned(_ os.FileInfo, _ int) bool { return false }
