package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestRepositorySatisfiesAdmittedSessionLister pins the binding between the
// admission controller's population source and the repository method that backs
// it. The controller reaches its repository through a narrow consumer-side
// interface and a type assertion, following the in-tree idiom
// (lifecycleTaskMetadataLister). That idiom degrades silently: a renamed or
// re-signatured repository method does not break the build, it just makes the
// assertion fail at runtime and leaves the controller permanently unable to read
// the population. For every other consumer that means a skipped sweep; here it
// would mean the ceiling never counts persisted sessions at all. This
// compile-time assertion turns that into a build failure instead.
func TestRepositorySatisfiesAdmittedSessionLister(t *testing.T) {
	var _ admittedSessionLister = (*sqlite.Repository)(nil)
}
