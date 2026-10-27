package backendapp

import (
	"path/filepath"
	"strings"
)

// deniedTemporaryRoots are the darwin locations the sandbox never grants.
var deniedTemporaryRoots = []string{"/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}

// refuseDeniedStateRoot rejects a darwin runtime base inside a temporary location
// that sandboxed children must not reach.
func refuseDeniedStateRoot(hostOS, base string) error {
	if hostOS != hostDarwin {
		return nil
	}
	clean := filepath.Clean(base)
	for _, root := range deniedTemporaryRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return errSandboxRuntimeUnavailable
		}
	}
	return nil
}
