package scheduler

import (
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
)

func TestConvertChangeToMutation_PropagatesSkipAssigneeCommentWake(t *testing.T) {
	got := convertChangeToMutation(dashboard.TaskReactivityChange{
		Comment: &dashboard.TaskReactivityComment{
			ID:         "comment-1",
			Body:       "@Reviewer please look",
			AuthorType: "user",
			AuthorID:   "user-1",
		},
		SkipAssigneeCommentWake: true,
	})

	if got.Comment == nil {
		t.Fatal("Comment = nil, want converted comment")
	}
	if !got.Comment.SkipAssigneeWake {
		t.Fatal("SkipAssigneeWake = false, want true")
	}
}
