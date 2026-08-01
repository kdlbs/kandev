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

func validateTaskTitle(title string) error {
	if utf8.RuneCountInString(title) <= TaskTitleMaxLength {
		return nil
	}
	return fmt.Errorf("%w: task titles must be %d characters or fewer", ErrTaskTitleTooLong, TaskTitleMaxLength)
}
