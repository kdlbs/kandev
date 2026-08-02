package service

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// TaskTitleMaxLength is the maximum number of characters allowed in a new or
// replacement task title.
const TaskTitleMaxLength = 60

// ErrTaskTitleTooLong identifies a task title that exceeds TaskTitleMaxLength.
var ErrTaskTitleTooLong = errors.New("task title is too long")

// ValidateTaskTitle checks whether title fits the task title character limit.
func ValidateTaskTitle(title string) error {
	if utf8.RuneCountInString(title) <= TaskTitleMaxLength {
		return nil
	}
	return fmt.Errorf("%w: task titles must be %d characters or fewer", ErrTaskTitleTooLong, TaskTitleMaxLength)
}

// TruncateTaskTitle shortens title to the task title limit, reserving the last
// character for an ellipsis when truncation is required.
func TruncateTaskTitle(title string) string {
	characters := []rune(title)
	if len(characters) <= TaskTitleMaxLength {
		return title
	}
	return string(characters[:TaskTitleMaxLength-1]) + "…"
}

func validateTaskTitle(title string) error {
	return ValidateTaskTitle(title)
}
