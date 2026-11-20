package journal

import (
	"errors"
	"io/fs"
	"time"

	bolt "go.etcd.io/bbolt"
)

// OpenReason is the closed source-owned classification of a journal
// acquisition failure. It describes the acquisition attempt only; it never
// claims a lock holder's identity.
type OpenReason string

const (
	OpenReasonLockTimeout       OpenReason = "lock_timeout"
	OpenReasonPermissionDenied  OpenReason = "permission_denied"
	OpenReasonFilesystemFull    OpenReason = "filesystem_full"
	OpenReasonDiskQuotaExceeded OpenReason = "disk_quota_exceeded"
	OpenReasonIOFailure         OpenReason = "io_failure"
	OpenReasonCorrupt           OpenReason = "corrupt"
	OpenReasonNewerVersion      OpenReason = "newer_version"
	OpenReasonUnavailable       OpenReason = "unavailable"
)

// OpenError wraps a failed journal acquisition with a bounded, source-owned
// reason. Error() is safe to project because it never includes the path, the
// wrapped cause, or journal content. Unwrap retains the underlying cause so
// errors.Is keeps working for bolt.ErrTimeout, permission errors, and
// filesystem errors.
type OpenError struct {
	reason  OpenReason
	path    string
	timeout time.Duration
	elapsed time.Duration
	cause   error
}

func (e *OpenError) Error() string {
	if e == nil {
		return "open delivery journal: " + string(OpenReasonUnavailable)
	}
	return "open delivery journal: " + string(e.reason)
}

func (e *OpenError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Reason returns the closed acquisition classification.
func (e *OpenError) Reason() OpenReason {
	if e == nil {
		return OpenReasonUnavailable
	}
	return e.reason
}

// Path returns the trusted configured journal path for internal diagnostics.
// It must not be projected to a browser or task API.
func (e *OpenError) Path() string {
	if e == nil {
		return ""
	}
	return e.path
}

// ConfiguredTimeout returns the lock timeout used for the acquisition attempt.
func (e *OpenError) ConfiguredTimeout() time.Duration {
	if e == nil {
		return 0
	}
	return e.timeout
}

// Elapsed returns how long the failed acquisition attempt took.
func (e *OpenError) Elapsed() time.Duration {
	if e == nil {
		return 0
	}
	return e.elapsed
}

func newOpenError(path string, timeout, elapsed time.Duration, cause error) *OpenError {
	return &OpenError{
		reason:  classifyOpenFailure(cause),
		path:    path,
		timeout: timeout,
		elapsed: elapsed,
		cause:   cause,
	}
}

func classifyOpenFailure(err error) OpenReason {
	switch {
	case err == nil:
		return OpenReasonUnavailable
	case errors.Is(err, bolt.ErrTimeout):
		return OpenReasonLockTimeout
	case errors.Is(err, fs.ErrPermission):
		return OpenReasonPermissionDenied
	case errors.Is(err, ErrJournalNewerVersion):
		return OpenReasonNewerVersion
	case errors.Is(err, ErrJournalCorrupt),
		errors.Is(err, bolt.ErrInvalid),
		errors.Is(err, bolt.ErrChecksum),
		errors.Is(err, bolt.ErrVersionMismatch):
		return OpenReasonCorrupt
	}
	if reason := classifyOpenFilesystemCause(err); reason != "" {
		return reason
	}
	return OpenReasonUnavailable
}

func (r OpenReason) String() string { return string(r) }

// journalErrorLabelForOpenReason maps the closed acquisition reason onto the
// bounded metric label set.
func journalErrorLabelForOpenReason(reason OpenReason) string {
	switch reason {
	case OpenReasonCorrupt:
		return "journal_corrupt"
	case OpenReasonNewerVersion:
		return "journal_newer_version"
	case OpenReasonUnavailable:
		return "storage_unavailable"
	case OpenReasonLockTimeout:
		return "lock_timeout"
	case OpenReasonPermissionDenied:
		return "permission_denied"
	case OpenReasonFilesystemFull:
		return "filesystem_full"
	case OpenReasonDiskQuotaExceeded:
		return "disk_quota_exceeded"
	case OpenReasonIOFailure:
		return "io_failure"
	default:
		return journalErrorUnknownLabel
	}
}

func openErrorReason(err error) (OpenReason, bool) {
	var openErr *OpenError
	if errors.As(err, &openErr) {
		return openErr.Reason(), true
	}
	return "", false
}
