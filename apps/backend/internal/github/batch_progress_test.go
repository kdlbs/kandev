package github

import (
	"testing"
	"time"
)

func TestBatchedBranchQueryProgressExpiresAfterTTL(t *testing.T) {
	staleProgress := &batchedBranchQueryProgress{}
	service := &Service{
		batchedBranchProgress: map[string]*batchedBranchProgressEntry{
			"query": {
				progress: staleProgress,
				updated:  time.Now().Add(-batchedPRProgressTTL - time.Second),
			},
		},
	}

	current := service.batchedBranchQueryProgress("query")
	if current == staleProgress {
		t.Fatal("branch query reused an expired continuation")
	}
	service.finishBatchedBranchQueryProgress("query", current, false)
}
