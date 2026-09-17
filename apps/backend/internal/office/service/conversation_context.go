package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Native conversations retain their full comment history in the database, but
// only a bounded recent window is sent to each fresh provider conversation.
func (si *SchedulerIntegration) conversationContext(ctx context.Context, taskID string) string {
	comments, err := si.svc.repo.ListRecentTaskComments(ctx, taskID, 4)
	if err != nil || len(comments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Recent conversation (bounded; retrieve older task comments only when needed):\n")
	for i := len(comments) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "%s: %s\n", comments[i].AuthorType, clipConversationText(comments[i].Body, 1000))
	}
	return b.String()
}

func clipConversationText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + "\n[Excerpt; full content remains in the task conversation.]"
}
