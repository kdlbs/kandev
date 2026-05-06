package routingerr

import "os"

// defaultOsEnviron returns the live host environment. Split into a
// file-private helper so tests can stub osEnviron without picking up
// real machine state.
func defaultOsEnviron() []string {
	return os.Environ()
}
