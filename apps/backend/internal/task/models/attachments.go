package models

import "errors"

const (
	MaxMessageAttachmentBytes int64 = 100 * 1024 * 1024
	MaxMessageAttachmentCount       = 10
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
