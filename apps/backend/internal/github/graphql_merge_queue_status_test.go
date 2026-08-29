package github

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPRFieldsBlockRequestsMergeQueueEntry(t *testing.T) {
	block := prFieldsBlock()
	if !strings.Contains(block, "mergeQueueEntry { id state position estimatedTimeToMerge headCommit { oid } }") {
		t.Fatalf("prFieldsBlock() = %s, want merge queue fields", block)
	}
}

func TestConvertBatchedPRResultPreservesMergeQueueObservation(t *testing.T) {
	tests := []struct {
		name         string
		entry        string
		wantState    string
		wantPosition *int
		wantEstimate *int
		wantObserved bool
	}{
		{
			name:         "queued with metadata",
			entry:        `{"id":"entry-a","state":"QUEUED","position":3,"estimatedTimeToMerge":125,"headCommit":{"oid":"head-a"}}`,
			wantState:    "queued",
			wantPosition: intPtr(3),
			wantEstimate: intPtr(125),
			wantObserved: true,
		},
		{
			name:         "active entry without estimate",
			entry:        `{"state":"MERGEABLE","position":1,"estimatedTimeToMerge":null}`,
			wantState:    "mergeable",
			wantPosition: intPtr(1),
			wantObserved: true,
		},
		{
			name:         "queue exit",
			entry:        `null`,
			wantObserved: true,
		},
		{
			name:         "unknown future state",
			entry:        `{"state":"PAUSED","position":7,"estimatedTimeToMerge":60}`,
			wantState:    "paused",
			wantPosition: intPtr(7),
			wantEstimate: intPtr(60),
			wantObserved: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := &batchedPRResult{}
			payload := []byte(`{"state":"OPEN","mergeQueueEntry":` + test.entry + `}`)
			if err := json.Unmarshal(payload, raw); err != nil {
				t.Fatalf("decode GraphQL payload: %v", err)
			}
			status := convertBatchedPRResult(raw, "owner", "repo", 42)
			if status.MergeQueueState != test.wantState {
				t.Errorf("MergeQueueState = %q, want %q", status.MergeQueueState, test.wantState)
			}
			if !intPtrEqual(status.MergeQueuePosition, test.wantPosition) {
				t.Errorf("MergeQueuePosition = %v, want %v", status.MergeQueuePosition, test.wantPosition)
			}
			if !intPtrEqual(status.MergeQueueEstimatedTimeToMergeSeconds, test.wantEstimate) {
				t.Errorf("MergeQueueEstimatedTimeToMergeSeconds = %v, want %v", status.MergeQueueEstimatedTimeToMergeSeconds, test.wantEstimate)
			}
			if test.name == "queued with metadata" {
				if status.MergeQueueEntryID != "entry-a" || status.MergeQueueEntryHeadSHA != "head-a" {
					t.Errorf("merge queue entry = %q/%q, want entry-a/head-a", status.MergeQueueEntryID, status.MergeQueueEntryHeadSHA)
				}
			}
			if status.mergeQueuePopulated != test.wantObserved {
				t.Errorf("mergeQueuePopulated = %v, want %v", status.mergeQueuePopulated, test.wantObserved)
			}
		})
	}
}
