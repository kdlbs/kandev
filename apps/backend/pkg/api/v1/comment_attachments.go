package v1

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var commentAttachmentMarker = regexp.MustCompile(`(?m)^<!-- kandev-comment-files:([a-zA-Z0-9_-]{1,128}) -->$`)

func commentAttachmentMarkers(content string) [][]int {
	return commentAttachmentMarker.FindAllStringSubmatchIndex(content, -1)
}

func ValidateCommentAttachments(content string, files []MessageAttachment) error {
	comments := make(map[string]int)
	for _, file := range files {
		if file.CommentID == "" {
			continue
		}
		if file.AttachmentID == "" || file.Data != "" || file.DeliveryMode != attachmentDeliveryPath {
			return fmt.Errorf("comment files require a file-backed path attachment")
		}
		comments[file.CommentID] = 0
	}
	if len(comments) == 0 {
		return nil
	}
	for _, marker := range commentAttachmentMarkers(content) {
		id := content[marker[2]:marker[3]]
		count, exists := comments[id]
		if !exists {
			continue
		}
		if count != 0 {
			return fmt.Errorf("unresolved comment files")
		}
		comments[id]++
	}
	for _, count := range comments {
		if count != 1 {
			return fmt.Errorf("unresolved comment files")
		}
	}
	return nil
}

func HasCommentAttachments(files []MessageAttachment) bool {
	for _, file := range files {
		if file.CommentID != "" {
			return true
		}
	}
	return false
}

func RenderCommentFiles(prompt string, files []MessageAttachment) (string, []MessageAttachment, error) {
	if !HasCommentAttachments(files) {
		return prompt, files, nil
	}
	if err := ValidateCommentAttachments(prompt, files); err != nil {
		return "", nil, err
	}
	markers := commentAttachmentMarkers(prompt)
	if len(markers) == 0 {
		return prompt, files, nil
	}
	paths := make(map[string][]string, len(markers))
	remaining := make([]MessageAttachment, 0, len(files))
	for _, file := range files {
		if file.CommentID == "" {
			remaining = append(remaining, file)
			continue
		}
		if strings.TrimSpace(file.Path) == "" {
			return "", nil, fmt.Errorf("unresolved comment files")
		}
		paths[file.CommentID] = append(paths[file.CommentID],
			"- "+strconv.Quote(file.Name)+": "+strconv.Quote(file.Path))
	}

	var out strings.Builder
	offset := 0
	for _, marker := range markers {
		pathsForComment, matched := paths[prompt[marker[2]:marker[3]]]
		if !matched {
			continue
		}
		out.WriteString(prompt[offset:marker[0]])
		out.WriteString(strings.Join(pathsForComment, "\n"))
		offset = marker[1]
	}
	out.WriteString(prompt[offset:])
	return out.String(), remaining, nil
}
