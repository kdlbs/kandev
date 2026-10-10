package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

const durableJournalContainerRoot = "/var/lib/kandev/agentctl-journals"

func resolveDurableJournal(req *ExecutorCreateRequest) (journal.Location, error) {
	if req == nil {
		return journal.Location{}, fmt.Errorf("executor request is required")
	}
	location, err := journal.ResolveLocation(req.DurableJournalHostRoot, req.DurableJournalOwnerID)
	if err != nil {
		return journal.Location{}, err
	}
	capability := journal.CheckStorage(location.Root, location.OwnerID)
	if !capability.Durable {
		return journal.Location{}, fmt.Errorf("retained journal ownership unavailable: %s", capability.Reason)
	}
	return location, nil
}

func durableJournalContainerPath(req *ExecutorCreateRequest) (string, error) {
	location, err := resolveDurableJournal(req)
	if err != nil {
		return "", err
	}
	return filepath.Join(durableJournalContainerRoot, location.OwnerID, "delivery.bbolt"), nil
}
