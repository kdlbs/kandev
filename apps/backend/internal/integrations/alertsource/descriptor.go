package alertsource

import (
	"fmt"
	"reflect"
)

// Capabilities declares which of a Source's optional behaviors are active.
// A struct of three independent bools rather than a bitmask: the axes are
// orthogonal, the zero value ("nothing declared") is itself meaningful, and
// it needs no String() or mask arithmetic (D18).
type Capabilities struct {
	Webhook bool
	Poll    bool
	Enrich  bool
}

// fieldCapabilities is the FieldError.Field value for all five capability
// codes (MissingCapability plus the four CheckCapabilities codes) — none of
// them names a sub-part of Capabilities, so one shared path is correct, not
// merely deduplicated.
const fieldCapabilities = "Capabilities"

// Descriptor is a Source's static declaration: identity, the field spec
// governing its configuration, how it participates in dedup
// (FingerprintFields) and per-watch throttling (WatchMetadataKey), and
// which optional interfaces it implements (Capabilities). DisplayName and
// Category are deliberately unvalidated — UI text with no consumer that can
// break.
type Descriptor struct {
	Type              string
	DisplayName       string
	Category          string
	WatchMetadataKey  string
	FingerprintFields []string
	Config            *FieldSpec
	Capabilities      Capabilities
}

// Validate checks the descriptor's own declaration for internal
// consistency. It reports MissingCapability itself — the one capability
// code it can reach without a Source, since the check compares two declared
// booleans (A1); the other three capability codes belong to
// CheckCapabilities, which alone can see an implementation. Errors from
// Config.Validate() are merged in under a "Config."-prefixed Field path so
// a caller has one flat sorted list to render. Same four rules as
// FieldSpec.Validate: aggregate (not fail-fast), sort by
// Field->Code->Message, never leak a value in Message, plain nil on
// success.
func (d Descriptor) Validate() error {
	var errs []FieldError
	if d.Type == "" {
		errs = append(errs, FieldError{
			Field:   "Type",
			Code:    ErrCodeEmptyType,
			Message: "type must not be empty",
		})
	}
	errs = append(errs, validateWatchMetadataKey(d.WatchMetadataKey)...)
	errs = append(errs, validateFingerprintFields(d.FingerprintFields)...)
	errs = append(errs, validateDescriptorConfig(d.Config)...)
	if !d.Capabilities.Webhook && !d.Capabilities.Poll {
		errs = append(errs, FieldError{
			Field:   fieldCapabilities,
			Code:    ErrCodeMissingCapability,
			Message: "at least one of Webhook or Poll must be declared",
		})
	}
	return newSpecError(errs)
}

func validateWatchMetadataKey(key string) []FieldError {
	if key == "" {
		return []FieldError{{
			Field:   "WatchMetadataKey",
			Code:    ErrCodeMissingWatchMetadataKey,
			Message: "watch metadata key must not be empty",
		}}
	}
	if !isIdentifier(key) {
		return []FieldError{{
			Field:   "WatchMetadataKey",
			Code:    ErrCodeInvalidWatchMetadataKey,
			Message: "watch metadata key must be restricted to [A-Za-z0-9_]",
		}}
	}
	return nil
}

// validateFingerprintFields checks FingerprintFields per D17: empty
// entirely -> EmptyFingerprintFields; an empty-string entry ->
// EmptyFingerprintField at its own index; a duplicate key ->
// DuplicateFingerprintField, reported once per repeated OCCURRENCE after
// the first, anchored at that occurrence's own index. ["a","a","a"] yields
// exactly two errors, at FingerprintFields[1] and FingerprintFields[2] —
// the first occurrence is never flagged, because it duplicates nothing
// before it.
func validateFingerprintFields(fields []string) []FieldError {
	if len(fields) == 0 {
		return []FieldError{{
			Field:   "FingerprintFields",
			Code:    ErrCodeEmptyFingerprintFields,
			Message: "fingerprint fields must not be empty",
		}}
	}
	var errs []FieldError
	seen := make(map[string]bool, len(fields))
	for i, f := range fields {
		path := fmt.Sprintf("FingerprintFields[%d]", i)
		switch {
		case f == "":
			errs = append(errs, FieldError{
				Field:   path,
				Code:    ErrCodeEmptyFingerprintField,
				Message: "fingerprint field must not be empty",
			})
		case seen[f]:
			errs = append(errs, FieldError{
				Field:   path,
				Code:    ErrCodeDuplicateFingerprintField,
				Message: "fingerprint field declared more than once",
			})
		default:
			seen[f] = true
		}
	}
	return errs
}

// validateDescriptorConfig reports NilConfig without recursing when cfg is
// nil (a source with no declared configuration declares an empty FieldSpec
// instead), otherwise merges cfg.Validate()'s errors under a
// "Config."-prefixed Field path.
func validateDescriptorConfig(cfg *FieldSpec) []FieldError {
	if cfg == nil {
		return []FieldError{{
			Field:   "Config",
			Code:    ErrCodeNilConfig,
			Message: "config must not be nil",
		}}
	}
	err := cfg.Validate()
	if err == nil {
		return nil
	}
	specErr, ok := err.(*SpecError)
	if !ok {
		return nil
	}
	merged := make([]FieldError, len(specErr.Errors))
	for i, fe := range specErr.Errors {
		merged[i] = FieldError{
			Field:   "Config." + fe.Field,
			Code:    fe.Code,
			Message: fe.Message,
		}
	}
	return merged
}

// CheckCapabilities checks that d's declared Capabilities agree, in BOTH
// directions, with the interfaces s actually implements (D18). Poll: true
// without s implementing Poller is CapabilityNotImplemented (the watch's
// poll interval is configured and nothing ever polls); s implementing
// Poller with Poll: false is CapabilityNotDeclared (the frozen control flow
// skips undeclared sources, so an implemented method nobody calls is almost
// certainly a typo in the declaration). Both directions apply to Enrich the
// same way. Webhook has no interface to compare against — every Source
// implements Normalize — so it is declaration-only.
//
// A nil s (untyped nil, or a typed nil pointer boxed into a non-nil Source
// interface value, e.g. (*fakeSource)(nil)) is NilSource: the fourth
// capability code, distinct from the three agreement codes because those
// describe a disagreement between a declaration and an implementation, and
// none of them can describe the absence of the implementation itself.
func CheckCapabilities(d Descriptor, s Source) error {
	if isNilInterfaceValue(s) {
		return newSpecError([]FieldError{{
			Field:   fieldCapabilities,
			Code:    ErrCodeNilSource,
			Message: "source must not be nil",
		}})
	}
	var errs []FieldError
	_, isPoller := s.(Poller)
	errs = append(errs, capabilityAgreement(d.Capabilities.Poll, isPoller)...)
	_, isEnricher := s.(Enricher)
	errs = append(errs, capabilityAgreement(d.Capabilities.Enrich, isEnricher)...)
	return newSpecError(errs)
}

func capabilityAgreement(declared, implemented bool) []FieldError {
	switch {
	case declared && !implemented:
		return []FieldError{{
			Field:   fieldCapabilities,
			Code:    ErrCodeCapabilityNotImplemented,
			Message: "capability declared but not implemented",
		}}
	case !declared && implemented:
		return []FieldError{{
			Field:   fieldCapabilities,
			Code:    ErrCodeCapabilityNotDeclared,
			Message: "capability implemented but not declared",
		}}
	default:
		return nil
	}
}

// isNilInterfaceValue reports whether s is nil, either as an untyped nil
// interface value or as a typed nil (pointer, map, slice, chan or func)
// boxed into a non-nil interface value. s == nil alone misses the typed
// case — (*fakeSource)(nil) is a non-nil Source under s == nil and panics
// on the first method call instead of reporting NilSource (D18).
func isNilInterfaceValue(s Source) bool {
	if s == nil {
		return true
	}
	v := reflect.ValueOf(s)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
