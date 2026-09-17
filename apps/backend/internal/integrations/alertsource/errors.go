// Package alertsource declares an external alert source as data rather than
// code: one normalized Alert model, three shared tables, and a descriptor
// with a chained field-spec builder that yields both config validation and
// an emitted JSON Schema from a single declaration.
package alertsource

import (
	"fmt"
	"sort"
	"strings"
)

// ErrorCode identifies a specific configuration or declaration defect. The
// set is closed and every value is pinned lower snake_case, distinct from
// the Go identifier, matching the repo's convention for closed code sets
// (routingerr.Code, RepositoryProviderErrorCode, IssueCode, ProbeFailureCode).
type ErrorCode string

// Config codes, reported by ValidationError.
const (
	ErrCodeUnknownField    ErrorCode = "unknown_field"
	ErrCodeMissingRequired ErrorCode = "missing_required"
	ErrCodeWrongType       ErrorCode = "wrong_type"
	ErrCodeInvalidValue    ErrorCode = "invalid_value"
)

// Declaration codes, field-level. Reported by SpecError.
const (
	ErrCodeDuplicateField      ErrorCode = "duplicate_field"
	ErrCodeEmptyFieldName      ErrorCode = "empty_field_name"
	ErrCodeInvalidFieldName    ErrorCode = "invalid_field_name"
	ErrCodeNilField            ErrorCode = "nil_field"
	ErrCodeSecretHasLiteral    ErrorCode = "secret_has_literal"
	ErrCodeSecretNonStringKind ErrorCode = "secret_non_string_kind"
	ErrCodeDefaultKindMismatch ErrorCode = "default_kind_mismatch"
	ErrCodeExampleKindMismatch ErrorCode = "example_kind_mismatch"
)

// Declaration codes, call-level. Reported by SpecError.
const (
	ErrCodeNilSpec           ErrorCode = "nil_spec"
	ErrCodeNilResolver       ErrorCode = "nil_resolver"
	ErrCodeInvalidConfigMode ErrorCode = "invalid_config_mode"
	ErrCodeEmptySourceID     ErrorCode = "empty_source_id"
)

// Declaration codes, descriptor-level. Reported by SpecError.
//
// MissingCapability is produced by Descriptor.Validate() (it compares two
// declared booleans and needs no Source). The other three are produced by
// CheckCapabilities, which alone can see an implementation.
const (
	ErrCodeEmptyType                 ErrorCode = "empty_type"
	ErrCodeMissingWatchMetadataKey   ErrorCode = "missing_watch_metadata_key"
	ErrCodeInvalidWatchMetadataKey   ErrorCode = "invalid_watch_metadata_key"
	ErrCodeEmptyFingerprintFields    ErrorCode = "empty_fingerprint_fields"
	ErrCodeDuplicateFingerprintField ErrorCode = "duplicate_fingerprint_field"
	ErrCodeEmptyFingerprintField     ErrorCode = "empty_fingerprint_field"
	ErrCodeNilConfig                 ErrorCode = "nil_config"
	ErrCodeMissingCapability         ErrorCode = "missing_capability"
	ErrCodeCapabilityNotImplemented  ErrorCode = "capability_not_implemented"
	ErrCodeCapabilityNotDeclared     ErrorCode = "capability_not_declared"
	ErrCodeNilSource                 ErrorCode = "nil_source"
)

// allErrorCodes is the closed set. A code added later without an entry here
// fails the determinism/closed-set test rather than silently emitting "".
var allErrorCodes = []ErrorCode{
	ErrCodeUnknownField, ErrCodeMissingRequired, ErrCodeWrongType, ErrCodeInvalidValue,
	ErrCodeDuplicateField, ErrCodeEmptyFieldName, ErrCodeInvalidFieldName, ErrCodeNilField,
	ErrCodeSecretHasLiteral, ErrCodeSecretNonStringKind, ErrCodeDefaultKindMismatch, ErrCodeExampleKindMismatch,
	ErrCodeNilSpec, ErrCodeNilResolver, ErrCodeInvalidConfigMode, ErrCodeEmptySourceID,
	ErrCodeEmptyType, ErrCodeMissingWatchMetadataKey, ErrCodeInvalidWatchMetadataKey,
	ErrCodeEmptyFingerprintFields, ErrCodeDuplicateFingerprintField, ErrCodeEmptyFingerprintField,
	ErrCodeNilConfig, ErrCodeMissingCapability, ErrCodeCapabilityNotImplemented,
	ErrCodeCapabilityNotDeclared, ErrCodeNilSource,
}

// FieldError names a single defect: the declaration path or config key it
// was found at, the closed code, and a human message that never contains the
// offending value (a Secret's literal must never round-trip into an error).
type FieldError struct {
	Field   string
	Code    ErrorCode
	Message string
}

func (e FieldError) String() string {
	return fmt.Sprintf("%s: %s: %s", e.Field, e.Code, e.Message)
}

// ValidationError reports one or more bad configuration values.
type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string { return joinFieldErrors(e.Errors) }

// SpecError reports one or more bad declarations, or a bad call.
type SpecError struct {
	Errors []FieldError
}

func (e *SpecError) Error() string { return joinFieldErrors(e.Errors) }

func joinFieldErrors(errs []FieldError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.String()
	}
	return strings.Join(parts, "; ")
}

// sortFieldErrors gives FieldError slices a total, deterministic order:
// Field, then Code, then Message. Message is what breaks a tie between two
// entries sharing a Field and a Code (e.g. two nil *Field entries, both
// recorded as {"", NilField, ...}) — without it sort.Slice's lack of
// stability would make the ordering flaky rather than merely unspecified.
func sortFieldErrors(errs []FieldError) {
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Field != errs[j].Field {
			return errs[i].Field < errs[j].Field
		}
		if errs[i].Code != errs[j].Code {
			return errs[i].Code < errs[j].Code
		}
		return errs[i].Message < errs[j].Message
	})
}

// newValidationError returns nil on an empty slice rather than a typed-nil
// *ValidationError: a nil *ValidationError boxed into the error interface is
// a non-nil error, which would fail every valid config (D10).
func newValidationError(errs []FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	sortFieldErrors(errs)
	return &ValidationError{Errors: errs}
}

// newSpecError is newValidationError's declaration-error counterpart.
func newSpecError(errs []FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	sortFieldErrors(errs)
	return &SpecError{Errors: errs}
}
