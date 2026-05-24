package github

import "errors"

// ErrInvalidPRURL signals that a caller-supplied PR URL could not be parsed.
// Used by AssociateExistingPRByURL so HTTP callers can translate the failure
// into a 400 instead of a generic 500.
var ErrInvalidPRURL = errors.New("invalid PR URL")

// ErrTaskNotFound is the sentinel that cleanup paths check to distinguish
// "the task is already gone — fine, mop up the dedup row" from a real
// upstream failure. Adapter implementations of TaskDeleter wrap this when
// the task domain reports a missing row so the github layer can recognize
// the case without string-matching the underlying error message.
var ErrTaskNotFound = errors.New("github: task not found for cleanup")

// ErrSelfApprove is returned by SubmitReview when the authenticated user
// attempts to APPROVE their own PR. GitHub rejects this with a 422; we
// catch it server-side so the UI sees a clean, typed error rather than a
// generic upstream failure when the frontend's visibility guard is bypassed.
var ErrSelfApprove = errors.New("cannot approve your own pull request")

// ErrInvalidToken is returned by ConfigureToken when the supplied PAT fails
// validation against the GitHub API. Wrapped around the underlying client
// error so HTTP callers can distinguish a validation failure (HTTP 400)
// from a secret-store write failure (HTTP 500).
var ErrInvalidToken = errors.New("invalid token")
