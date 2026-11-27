//go:build cgo

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func seedReplaceFixture(t *testing.T) (*Repository, *models.TaskPreviewFeedback) {
	t.Helper()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, replaceWorkspace)
	seedAttachmentTask(t, repo, replaceTask, replaceWorkspace)
	seedReplaceAttachment(t, repo, "shot-old", replaceOwner)
	item := &models.TaskPreviewFeedback{
		ID: "replace-feedback", TaskID: replaceTask,
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "Clipped",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Checkout",
		PageRoute: "/checkout", PageTitle: "Checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: "shot-old",
	}
	if _, err := repo.CreateTaskPreviewFeedback(
		context.Background(), item, replaceOwner, replaceWorkspace,
	); err != nil {
		t.Fatal(err)
	}
	return repo, item
}

func attachmentState(t *testing.T, repo *Repository, id string) string {
	t.Helper()
	stored, err := repo.GetMessageAttachment(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return stored.State
}

func TestPreviewFeedbackReplaceScreenshotSwapsAttachmentAndBumpsVersion(t *testing.T) {
	repo, item := seedReplaceFixture(t)
	seedReplaceAttachment(t, repo, "shot-new", replaceOwner)

	snapshot, released, err := repo.ReplaceTaskPreviewFeedbackScreenshot(
		context.Background(), replacement("shot-new", 1), replaceOwner, replaceWorkspace,
	)
	if err != nil {
		t.Fatalf("ReplaceTaskPreviewFeedbackScreenshot: %v", err)
	}
	got := snapshot.Items[0]
	if snapshot.Revision != 2 || got.Version != 2 || got.ScreenshotAttachmentID != "shot-new" ||
		got.Comment != item.Comment || string(got.CaptureRect) != `{"x":10,"y":20,"width":200,"height":100}` {
		t.Fatalf("replaced snapshot = %#v item=%#v", snapshot, got)
	}
	if len(released) != 1 || released[0].ID != "shot-old" {
		t.Fatalf("released = %#v", released)
	}
	if attachmentState(t, repo, "shot-old") != models.AttachmentStateExpired ||
		attachmentState(t, repo, "shot-new") != models.AttachmentStateClaimed {
		t.Fatal("attachments were not swapped")
	}
}

func TestPreviewFeedbackReplaceScreenshotUpdatesOptionalComment(t *testing.T) {
	repo, _ := seedReplaceFixture(t)
	seedReplaceAttachment(t, repo, "shot-new", replaceOwner)
	change := replacement("shot-new", 1)
	change.Comment = "Now wider"

	snapshot, _, err := repo.ReplaceTaskPreviewFeedbackScreenshot(
		context.Background(), change, replaceOwner, replaceWorkspace,
	)
	if err != nil || snapshot.Items[0].Comment != "Now wider" {
		t.Fatalf("replace with comment = %#v, %v", snapshot, err)
	}
}

func TestPreviewFeedbackReplaceScreenshotRejectsWithoutChanges(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, repo *Repository) *models.TaskPreviewFeedbackScreenshotReplacement
		wantErr error
	}{
		{
			name: "stale version",
			setup: func(t *testing.T, repo *Repository) *models.TaskPreviewFeedbackScreenshotReplacement {
				seedReplaceAttachment(t, repo, "shot-new", replaceOwner)
				return replacement("shot-new", 7)
			},
			wantErr: repoerrors.ErrTaskPreviewFeedbackChanged,
		},
		{
			name: "foreign attachment",
			setup: func(t *testing.T, repo *Repository) *models.TaskPreviewFeedbackScreenshotReplacement {
				seedReplaceAttachment(t, repo, "shot-new", "someone-else")
				return replacement("shot-new", 1)
			},
			wantErr: models.ErrAttachmentClaimConflict,
		},
		{
			name: "already claimed attachment",
			setup: func(t *testing.T, _ *Repository) *models.TaskPreviewFeedbackScreenshotReplacement {
				return replacement("shot-old", 1)
			},
			wantErr: models.ErrAttachmentClaimConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, item := seedReplaceFixture(t)
			change := tc.setup(t, repo)
			_, _, err := repo.ReplaceTaskPreviewFeedbackScreenshot(
				context.Background(), change, replaceOwner, replaceWorkspace,
			)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			assertReplaceUnchanged(t, repo, item)
		})
	}
}

func TestPreviewFeedbackReplaceScreenshotRejectsNonScreenshotItem(t *testing.T) {
	repo, _ := seedReplaceFixture(t)
	text := &models.TaskPreviewFeedback{
		ID: "replace-text", TaskID: replaceTask,
		Kind: models.TaskPreviewFeedbackText, Comment: "Wording",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Checkout",
		PageRoute: "/checkout", SelectedText: "Buy",
		TextAnchor: json.RawMessage(`{"start":{"path":[0],"offset":0}}`),
	}
	if _, err := repo.CreateTaskPreviewFeedback(context.Background(), text, "", ""); err != nil {
		t.Fatal(err)
	}
	seedReplaceAttachment(t, repo, "shot-new", replaceOwner)
	change := replacement("shot-new", 1)
	change.ItemID = text.ID

	_, _, err := repo.ReplaceTaskPreviewFeedbackScreenshot(
		context.Background(), change, replaceOwner, replaceWorkspace,
	)
	if !errors.Is(err, previewfeedback.ErrCaptureInvalid) {
		t.Fatalf("error = %v, want capture invalid", err)
	}
	if attachmentState(t, repo, "shot-new") != models.AttachmentStateStaged {
		t.Fatal("new attachment was claimed for a text item")
	}
}

func assertReplaceUnchanged(t *testing.T, repo *Repository, item *models.TaskPreviewFeedback) {
	t.Helper()
	snapshot, err := repo.ListTaskPreviewFeedback(context.Background(), replaceTask)
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.Items[0]
	if snapshot.Revision != 1 || got.Version != 1 || got.ScreenshotAttachmentID != "shot-old" ||
		string(got.CaptureRect) != string(item.CaptureRect) {
		t.Fatalf("item changed after rejected replace: %#v", got)
	}
	if attachmentState(t, repo, "shot-old") != models.AttachmentStateClaimed {
		t.Fatal("old attachment was released after rejected replace")
	}
}
