package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/planinjection"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	continuationSnapshotBudget     = 64 * 1024
	continuationReservedBudget     = 16 * 1024
	continuationConversationBudget = continuationSnapshotBudget - continuationReservedBudget
	continuationTruncationMarker   = "[Kandev: history item truncated]"
)

// ContinuationSnapshotInput is the trusted task context and canonical message
// history used when an operator explicitly starts a new native conversation.
type ContinuationSnapshotInput struct {
	TaskObjective     string
	Plan              string
	OriginalWorkspace string
	Messages          []*models.Message
}

// ComposedContinuationSnapshot is the bounded context sent to a new native
// conversation. Content is history, not a system instruction.
type ComposedContinuationSnapshot struct {
	Content         string
	SourceMessageID string
	ByteCount       int
	OmittedMessages int
	Truncated       bool
	ContentHash     string
}

// BuildContinuationSnapshot composes canonical Kandev data into a bounded,
// UTF-8-safe context. The plan is reduced once inside the reserved context
// budget. Newest complete messages have priority while output remains ordered.
func BuildContinuationSnapshot(input ContinuationSnapshotInput) ComposedContinuationSnapshot {
	plan, planReduced, _ := planinjection.Reduce(planinjection.ContainTags(input.Plan), planinjection.HandoverBudget)
	reserved := formatReservedContext(input.TaskObjective, plan, input.OriginalWorkspace)
	reserved, reservedTruncated := trimUTF8ToBytesWithMetadata(reserved, continuationReservedBudget)

	entries, omitted, truncated := composeMessages(input.Messages, continuationConversationBudget)
	content := reserved
	if content != "" && entries != "" {
		content += "\n\n"
	}
	content += entries
	if planReduced {
		truncated = true
	}
	content, contentTruncated := trimUTF8ToBytesWithMetadata(content, continuationSnapshotBudget)

	var sourceID string
	for index := len(input.Messages) - 1; index >= 0; index-- {
		if input.Messages[index] != nil && input.Messages[index].ID != "" {
			sourceID = input.Messages[index].ID
			break
		}
	}
	hash := sha256.Sum256([]byte(content))
	return ComposedContinuationSnapshot{
		Content:         content,
		SourceMessageID: sourceID,
		ByteCount:       len([]byte(content)),
		OmittedMessages: omitted,
		Truncated:       reservedTruncated || contentTruncated || truncated || omitted > 0,
		ContentHash:     hex.EncodeToString(hash[:]),
	}
}

func formatReservedContext(objective, plan, workspace string) string {
	parts := make([]string, 0, 3)
	if text := cleanContextText(objective); text != "" {
		parts = append(parts, "Task objective:\n"+text)
	}
	if text := cleanContextText(plan); text != "" {
		parts = append(parts, "Saved plan context:\n"+text)
	}
	if text := cleanContextText(workspace); text != "" {
		parts = append(parts, "Original agent workspace:\n"+text)
	}
	return strings.Join(parts, "\n\n")
}

func composeMessages(messages []*models.Message, budget int) (string, int, bool) {
	if budget <= 0 {
		return "", len(messages), len(messages) > 0
	}
	selected := make([]string, 0, len(messages))
	used := 0
	omitted := 0
	truncated := false
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || strings.TrimSpace(message.Content) == "" {
			continue
		}
		entry := formatMessage(message)
		if used+len([]byte(entry)) <= budget {
			selected = append(selected, entry)
			used += len([]byte(entry))
			continue
		}
		available := budget - used
		if available > len([]byte(continuationTruncationMarker))+8 {
			partial := trimUTF8ToBytes(entry, available-len([]byte(continuationTruncationMarker)))
			selected = append(selected, partial+continuationTruncationMarker)
		}
		omitted = index + 1
		truncated = true
		break
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return strings.Join(selected, "\n\n"), omitted, truncated
}

func formatMessage(message *models.Message) string {
	author := strings.TrimSpace(string(message.AuthorType))
	if author == "" {
		author = kubernetesStatusUnknown
	}
	return fmt.Sprintf("[%s]\n%s", author, cleanContextText(message.Content))
}

func cleanContextText(text string) string {
	return strings.TrimSpace(planinjection.ContainTags(text))
}

func trimUTF8ToBytes(text string, budget int) string {
	trimmed, _ := trimUTF8ToBytesWithMetadata(text, budget)
	return trimmed
}

func trimUTF8ToBytesWithMetadata(text string, budget int) (string, bool) {
	if budget <= 0 {
		return "", text != ""
	}
	if len([]byte(text)) <= budget {
		return text, false
	}
	trimmed := text
	for len([]byte(trimmed)) > budget {
		_, size := utf8.DecodeLastRuneInString(trimmed)
		if size == 0 {
			return "", true
		}
		trimmed = trimmed[:len(trimmed)-size]
	}
	return trimmed, true
}
