package plugins

import (
	"errors"
	"fmt"
)

var (
	ErrPackageIdentityMismatch      = errors.New("catalog package identity mismatch")
	ErrPublisherEvidenceUnavailable = errors.New("publisher evidence unavailable for installed version")
	ErrInstalledPackageMismatch     = errors.New("installed package does not match published package")
	ErrInstalledPackageUnreadable   = errors.New("installed package is unreadable")
	ErrInstalledPackageChanged      = errors.New("installed package changed during verification")
)

const (
	PublisherCodeCatalogUnavailable      = "catalog_unavailable"
	PublisherCodeSelectionStale          = "catalog_selection_stale"
	PublisherCodePackageIdentityMismatch = "package_identity_mismatch"
	PublisherCodeChanged                 = "publisher_changed"
	PublisherCodeEvidenceUnavailable     = "publisher_evidence_unavailable"
	PublisherCodeInstalledMismatch       = "installed_package_mismatch"
	PublisherCodeInstalledUnreadable     = "installed_package_unreadable"
	PublisherCodeInstalledChanged        = "installed_package_changed"
	PublisherCodeVerificationFailed      = "publisher_verification_failed"
)

type publisherOperationError struct {
	Code string
	Err  error
}

func (e *publisherOperationError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *publisherOperationError) Unwrap() error { return e.Err }

func publisherError(code string, err error) error {
	return &publisherOperationError{Code: code, Err: err}
}

// PublisherErrorCode returns the stable machine-readable code for a publisher
// operation error, or an empty string for unrelated errors.
func PublisherErrorCode(err error) string {
	var operationErr *publisherOperationError
	if errors.As(err, &operationErr) {
		return operationErr.Code
	}
	return ""
}
