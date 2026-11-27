package service

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

const taskFlagNoteMaxLength = 1024

// ErrTaskFlagNoteTooLong identifies a task flag note that exceeds the size limit.
var ErrTaskFlagNoteTooLong = errors.New("task flag note is too long")

func validateTaskFlagNote(note *string) error {
	if note == nil {
		return nil
	}

	units := 0
	for _, character := range strings.TrimSpace(*note) {
		units += utf16.RuneLen(character)
		if units > taskFlagNoteMaxLength {
			return fmt.Errorf("%w: flag notes must be %d UTF-16 code units or fewer", ErrTaskFlagNoteTooLong, taskFlagNoteMaxLength)
		}
	}
	return nil
}
