package sqlite_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGetChildSummaries_ExcludesArchived(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "live", "ws-1", "Live child", "", "KAN-2")
	insertTask(t, repo, ctx, "gone", "ws-1", "Archived child", "", "KAN-3")
	mustExec(t, repo,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'COMPLETED' WHERE id IN ('live','gone')`)
	mustExec(t, repo,
		`UPDATE tasks SET archived_at = datetime('now') WHERE id = 'gone'`)

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if truncated {
		t.Error("truncated = true, want false")
	}
	if len(summaries) != 1 || summaries[0].TaskID != "live" {
		t.Fatalf("summaries = %+v, want only the live child", summaries)
	}
}

// A child that restarted between queue time and prompt assembly must still be
// listed, so the parent is shown the fact rather than a silently short list.
func TestGetChildSummaries_IncludesNonTerminalChild(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "restarted", "ws-1", "Restarted child", "", "KAN-2")
	mustExec(t, repo,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'IN_PROGRESS' WHERE id = 'restarted'`)

	summaries, _, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].State != "IN_PROGRESS" {
		t.Fatalf("summaries = %+v, want the IN_PROGRESS child", summaries)
	}
}

// The count and the rows must come from one statement over one predicate, so
// archived children can never raise the total past the cap while the capped
// select already covers every live child.
func TestGetChildSummaries_ArchivedChildCannotTriggerTruncation(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	for i := range 20 {
		id := fmt.Sprintf("live-%02d", i)
		insertTask(t, repo, ctx, id, "ws-1", id, "", "")
	}
	for i := range 5 {
		id := fmt.Sprintf("gone-%02d", i)
		insertTask(t, repo, ctx, id, "ws-1", id, "", "")
	}
	mustExec(t, repo, `UPDATE tasks SET parent_id = 'parent-1' WHERE id != 'parent-1'`)
	mustExec(t, repo,
		`UPDATE tasks SET archived_at = datetime('now') WHERE id LIKE 'gone-%'`)

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 20 {
		t.Fatalf("len(summaries) = %d, want 20", len(summaries))
	}
	if truncated {
		t.Error("truncated = true while every live child is listed")
	}
}

func TestGetChildSummaries_OrdersByCreatedAtThenID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	// c-b and c-c share a timestamp; c-a is older. Insertion order is
	// deliberately not the expected order.
	for _, c := range []struct{ id, createdAt string }{
		{"c-c", "2026-01-02 00:00:00"},
		{"c-a", "2026-01-01 00:00:00"},
		{"c-b", "2026-01-02 00:00:00"},
	} {
		insertTask(t, repo, ctx, c.id, "ws-1", c.id, "", "")
		mustExec(t, repo,
			`UPDATE tasks SET parent_id = 'parent-1', created_at = ? WHERE id = ?`, c.createdAt, c.id)
	}

	summaries, _, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	var got []string
	for _, s := range summaries {
		got = append(got, s.TaskID)
	}
	want := []string{"c-a", "c-b", "c-c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestGetChildSummaries_LastCommentTiebreak(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "child-1", "ws-1", "Child", "", "KAN-2")
	mustExec(t, repo, `UPDATE tasks SET parent_id = 'parent-1' WHERE id = 'child-1'`)
	// Same created_at: the higher comment id is the most recent.
	mustExec(t, repo,
		`INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		 VALUES ('c1', 'child-1', 'agent', 'a1', 'first', 'agent', '2026-01-01 00:00:00'),
		        ('c2', 'child-1', 'user', 'u1', 'second', 'user', '2026-01-01 00:00:00')`)

	summaries, _, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	if summaries[0].LastComment != "second" {
		t.Errorf("last comment = %q, want %q", summaries[0].LastComment, "second")
	}
}

func TestGetChildSummaries_CapsAtTwenty(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	for i := range 21 {
		id := fmt.Sprintf("child-%02d", i)
		insertTask(t, repo, ctx, id, "ws-1", id, "", "")
		mustExec(t, repo,
			`UPDATE tasks SET parent_id = 'parent-1', created_at = ? WHERE id = ?`,
			fmt.Sprintf("2026-01-01 00:00:%02d", i), id)
	}

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 20 {
		t.Fatalf("len(summaries) = %d, want 20", len(summaries))
	}
	if !truncated {
		t.Error("truncated = false, want true for 21 live children")
	}
	if summaries[0].TaskID != "child-00" || summaries[19].TaskID != "child-19" {
		t.Errorf("cap kept %s..%s, want child-00..child-19",
			summaries[0].TaskID, summaries[19].TaskID)
	}
}

// The renderer marks a body longer than the display limit. That branch is only
// reachable if the query reads one code point past the limit — and the count
// must be in code points, not bytes, or a multibyte body trips it early.
func TestGetChildSummaries_ReadsOneCodePointPastCommentCap(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")

	cases := []struct {
		name      string
		id        string
		body      string
		wantRunes int
	}{
		{"exactly at the limit", "at-limit", strings.Repeat("a", 500), 500},
		{"one past the limit", "over-limit", strings.Repeat("a", 501), 501},
		{"far past the limit", "way-over", strings.Repeat("a", 900), 501},
		{"multibyte at the limit", "cjk-at", strings.Repeat("界", 500), 500},
		{"multibyte past the limit", "cjk-over", strings.Repeat("界", 600), 501},
	}
	for _, tc := range cases {
		insertTask(t, repo, ctx, tc.id, "ws-1", tc.id, "", "")
		mustExec(t, repo, `UPDATE tasks SET parent_id = 'parent-1' WHERE id = ?`, tc.id)
		mustExec(t, repo,
			`INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
			 VALUES (?, ?, 'agent', 'a1', ?, 'agent', '2026-01-01 00:00:00')`,
			"cm-"+tc.id, tc.id, tc.body)
	}

	summaries, _, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	byID := map[string]string{}
	for _, s := range summaries {
		byID[s.TaskID] = s.LastComment
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := byID[tc.id]
			if n := utf8.RuneCountInString(got); n != tc.wantRunes {
				t.Errorf("comment length = %d code points, want %d", n, tc.wantRunes)
			}
			if !utf8.ValidString(got) {
				t.Error("comment is not valid UTF-8")
			}
		})
	}
}

// tasks.parent_id has no foreign key and no cascade, so child rows outlive a
// deleted parent. Membership must be decided by the parent existing, not by an
// absence of rows that reference it.
func TestGetChildSummaries_DeletedParentYieldsNoRows(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	for i := range 25 {
		id := fmt.Sprintf("orphan-%02d", i)
		insertTask(t, repo, ctx, id, "ws-1", id, "", "")
	}
	mustExec(t, repo, `UPDATE tasks SET parent_id = 'parent-1' WHERE id LIKE 'orphan-%'`)
	mustExec(t, repo, `DELETE FROM tasks WHERE id = 'parent-1'`)

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("len(summaries) = %d, want 0 for a deleted parent", len(summaries))
	}
	if truncated {
		t.Error("truncated = true for a deleted parent, want false")
	}
}
