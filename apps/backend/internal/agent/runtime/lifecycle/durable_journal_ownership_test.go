package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandaloneJournalLocationPersistsRetainedOwner(t *testing.T) {
	request := &ExecutorCreateRequest{DurableJournalHostRoot: t.TempDir(), DurableJournalOwnerID: "session"}
	location, err := resolveDurableJournal(request)
	require.NoError(t, err)
	owner, err := os.ReadFile(filepath.Join(filepath.Dir(location.Path), ".owner"))
	require.NoError(t, err, "the retained control API requires the launcher's persisted owner")
	require.Equal(t, "session\n", string(owner))
}
