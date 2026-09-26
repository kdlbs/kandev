package github

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.1
func TestPRFieldsBlockRequestsMergeQueueRecovery(t *testing.T) {
	block := prFieldsBlock()
	for _, want := range []string{
		"headRefOid",
		"mergeQueueEntry { id state position estimatedTimeToMerge headCommit { oid } }",
		"mergeQueueRemovalEvents: timelineItems(last: 1, itemTypes: REMOVED_FROM_MERGE_QUEUE_EVENT)",
		"beforeCommit { oid }",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("prFieldsBlock() missing %q: %s", want, block)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.1
func TestConvertBatchedPRResultPreservesMergeQueueRecovery(t *testing.T) {
	raw := &batchedPRResult{}
	payload := []byte(`{
		"state":"OPEN",
		"headRefOid":"head-a",
		"mergeQueueEntry":{
			"id":"entry-a",
			"state":"QUEUED",
			"position":2,
			"estimatedTimeToMerge":90,
			"headCommit":{"oid":"head-a"}
		},
		"mergeQueueRemovalEvents":{"nodes":[{
			"id":"removal-a",
			"createdAt":"2026-08-24T18:00:00Z",
			"reason":"CI checks failed",
			"beforeCommit":{"oid":"before-a"}
		}]}
	}`)
	if err := json.Unmarshal(payload, raw); err != nil {
		t.Fatalf("decode GraphQL payload: %v", err)
	}

	status := convertBatchedPRResult(raw, "owner", "repo", 42)
	if status.PR.HeadSHA != "head-a" {
		t.Fatalf("head SHA = %q, want head-a", status.PR.HeadSHA)
	}
	if status.MergeQueueEntryID != "entry-a" || status.MergeQueueEntryHeadSHA != "head-a" {
		t.Fatalf("active queue observation = %#v, want entry-a/head-a", status)
	}
	if status.MergeQueueLastRemovalID != "removal-a" {
		t.Fatalf("removal ID = %q, want removal-a", status.MergeQueueLastRemovalID)
	}
	if status.MergeQueueLastRemovedAt == nil || !status.MergeQueueLastRemovedAt.Equal(time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("removal time = %v, want 2026-08-24T18:00:00Z", status.MergeQueueLastRemovedAt)
	}
	if status.MergeQueueLastRemovalReason != "CI checks failed" || status.MergeQueueLastRemovalBeforeSHA != "before-a" {
		t.Fatalf("removal evidence = %#v, want reason and before SHA", status)
	}
	if !status.mergeQueueRecoveryPopulated {
		t.Fatal("merge queue recovery observation was not marked populated")
	}
}
