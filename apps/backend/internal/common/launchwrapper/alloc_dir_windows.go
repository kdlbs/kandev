//go:build windows

package launchwrapper

// ResolveAllocDir always refuses: there is no ownership model to verify here.
func ResolveAllocDir(string) (string, error) {
	return "", allocDirError("unsupported on this platform")
}
