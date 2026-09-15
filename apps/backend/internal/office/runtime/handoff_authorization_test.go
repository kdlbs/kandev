package runtime

import (
	"context"
	"errors"
	"testing"
)

// TestActionsHandoff_TasklessRunRefused proves a run with no owning task
// (runCtx.TaskID == "") is refused before any workspace scoping is
// attempted. An empty task ID has no owning identity to scope against;
// letting it reach Workspaces.Scope resolves to an unscoped caller, which
// the task service treats as an internal caller with owner rights on every
// workspace — bypassing the target-workspace ownership check entirely.
func TestActionsHandoff_TasklessRunRefused(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	scoper := deps.Workspaces.(*fakeHandoffScoper)
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	runCtx.TaskID = ""

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if !errors.Is(err, errHandoffTasklessRun) {
		t.Fatalf("error = %v, want errHandoffTasklessRun", err)
	}
	if len(scoper.calls) != 0 {
		t.Errorf("Workspaces.Scope was called (%v) despite a taskless run; must refuse before scoping", scoper.calls)
	}
	if len(tasks.createCalls) != 0 {
		t.Error("CreateTask was called despite a taskless run")
	}
}

// TestActionsHandoff_LaunchIndependentOfReverseLinkFailure proves the launch
// dispatch (AC-32) and the reverse-link write (AC-17) are independent
// outcomes: a reverse-link write failure must not prevent a requested launch
// from happening, and vice versa the launch's own success is unaffected by
// ReverseLinkRecorded being false.
func TestActionsHandoff_LaunchIndependentOfReverseLinkFailure(t *testing.T) {
	deps, _, reverseLinks, launcher, _ := baseHandoffDeps()
	reverseLinks.casErr = errors.New("cas store unavailable")
	actions := NewActions(ActionDependencies{Handoff: deps})
	req := baseHandoffRequest()
	start := true
	req.StartAgent = &start

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if result.ReverseLinkRecorded {
		t.Fatal("ReverseLinkRecorded = true, want false given a forced CAS error")
	}
	if result.ReverseLinkError == "" {
		t.Error("ReverseLinkError = \"\", want a non-empty message given a forced CAS error")
	}
	if !result.Started {
		t.Error("Started = false, want true: a reverse-link failure must not block the requested launch")
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher calls = %d, want 1", len(launcher.calls))
	}
}

// TestParseHandoffEntries is AC-27's exhaustive corruption-detection table:
// an absent handoffs key is empty-but-not-corrupt, while every other
// malformed shape is refused so a reverse-link append never silently drops
// or mangles sibling entries. A well-formed entry carrying unknown fields
// must survive byte-for-byte, since decode/re-encode through
// map[string]interface{} would silently corrupt additive numeric fields
// outside float64's exact integer range.
func TestParseHandoffEntries(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		deliveryTaskID string
		wantEntries    int
		wantPresent    bool
		wantCorrupt    bool
	}{
		{
			name:        "empty raw is absent, not corrupt",
			raw:         "",
			wantEntries: 0,
			wantCorrupt: false,
		},
		{
			name:        "whitespace-only raw is absent, not corrupt",
			raw:         "   ",
			wantEntries: 0,
			wantCorrupt: false,
		},
		{
			name:        "literal null is corrupt (present but not an array)",
			raw:         "null",
			wantCorrupt: true,
		},
		{
			name:        "non-array JSON is corrupt",
			raw:         `{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"}`,
			wantCorrupt: true,
		},
		{
			name:        "unparseable JSON is corrupt",
			raw:         `[{"task_id":`,
			wantCorrupt: true,
		},
		{
			name:        "array element that is not an object is corrupt",
			raw:         `[42]`,
			wantCorrupt: true,
		},
		{
			name:        "entry missing task_id is corrupt",
			raw:         `[{"task_id":"","handed_off_at":"2026-01-01T00:00:00.000Z"}]`,
			wantCorrupt: true,
		},
		{
			name:        "entry with unparseable handed_off_at is corrupt",
			raw:         `[{"task_id":"t1","handed_off_at":"not-a-timestamp"}]`,
			wantCorrupt: true,
		},
		{
			name:           "well-formed entries, delivery task not yet present",
			raw:            `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"},{"task_id":"t2","handed_off_at":"2026-01-02T00:00:00.000Z"}]`,
			deliveryTaskID: "t3",
			wantEntries:    2,
			wantPresent:    false,
		},
		{
			name:           "delivery task already present is reported, not duplicated",
			raw:            `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"}]`,
			deliveryTaskID: "t1",
			wantEntries:    1,
			wantPresent:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, present, corrupt := parseHandoffEntries(tt.raw, tt.deliveryTaskID)
			if (corrupt != "") != tt.wantCorrupt {
				t.Fatalf("corrupt = %q (non-empty=%v), want non-empty=%v", corrupt, corrupt != "", tt.wantCorrupt)
			}
			if tt.wantCorrupt {
				return
			}
			if len(entries) != tt.wantEntries {
				t.Errorf("len(entries) = %d, want %d", len(entries), tt.wantEntries)
			}
			if present != tt.wantPresent {
				t.Errorf("alreadyPresent = %v, want %v", present, tt.wantPresent)
			}
		})
	}
}

// TestParseHandoffEntries_UnknownFieldsSurviveByteForByte is AC-27: an entry
// carrying fields this code does not know about is well-formed and must be
// preserved as its original raw bytes, not reconstructed from the decoded
// task_id/handed_off_at pair alone.
func TestParseHandoffEntries_UnknownFieldsSurviveByteForByte(t *testing.T) {
	const raw = `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z","note":"kept as-is","big_number":9007199254740993}]`

	entries, present, corrupt := parseHandoffEntries(raw, "t1")
	if corrupt != "" {
		t.Fatalf("corrupt = %q, want empty", corrupt)
	}
	if !present {
		t.Fatal("alreadyPresent = false, want true")
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if got := string(entries[0].raw); got != `{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z","note":"kept as-is","big_number":9007199254740993}` {
		t.Errorf("raw bytes were not preserved unchanged: %s", got)
	}
}
