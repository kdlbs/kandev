package models

import (
	"errors"

	"github.com/kandev/kandev/internal/messageconstraints"
)

const (
	MaxMessageAttachmentBytes = messageconstraints.MaxAttachmentBytes
	MaxMessageAttachmentCount = messageconstraints.MaxAttachmentCount
)

var (
	ErrAttachmentTooLarge      = errors.New("attachment exceeds maximum size")
	ErrAttachmentTotalTooLarge = errors.New("total attachment size exceeds maximum")
	ErrTooManyAttachments      = errors.New("too many attachments")
	ErrAttachmentClaimConflict = errors.New("attachment claim conflict")
	ErrAttachmentNotFound      = errors.New("attachment not found")
	ErrAttachmentForbidden     = errors.New("attachment access denied")
	ErrAttachmentInvalid       = errors.New("invalid attachment")
)
