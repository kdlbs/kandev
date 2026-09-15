package alertsource

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/secrets"
)

// ConfigMode selects which requiredness rule ValidateConfig applies to a
// Secret field. Its zero value is ConfigCreate — the stricter mode — so a
// forgotten or zero-valued argument fails closed rather than silently
// accepting a config a real create would have rejected.
type ConfigMode int

const (
	// ConfigCreate requires every declared field, Secret or not.
	ConfigCreate ConfigMode = iota
	// ConfigUpdate additionally treats an absent Secret key as legal
	// (D6/D15): a writeOnly form field cannot round-trip a secret it was
	// never given, so its absence means "leave the stored value alone",
	// never "clear it". An absent non-secret required key is still
	// MissingRequired under this mode too.
	ConfigUpdate
)

const secretRedactionMarker = "SENTINEL-SECRET-VALUE"

// msgFieldRequired is ErrCodeMissingRequired's shared message, used at every
// site that reports an absent required field.
const msgFieldRequired = "field is required"

// SecretResolver reads a previously-stored secret by its store id. Declared
// locally so *secretadapter.Adapter satisfies it structurally without this
// package importing secretadapter.
type SecretResolver interface {
	Reveal(ctx context.Context, id string) (string, error)
}

// SecretWriter is T06's write-side seam (D15): Set upserts, Delete removes,
// and ListIDs enumerates for orphan cleanup after a source delete. Declared
// here, alongside SecretKey, so T06 does not invent a second spelling of the
// same seam; nothing in T04 calls it.
type SecretWriter interface {
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	ListIDs(ctx context.Context) ([]string, error)
}

// SecretKey is the one place the "alertsource:<source_id>:<field>" spelling
// exists, so LoadConfig's reads and T06's several write sites cannot drift
// apart.
func SecretKey(sourceID, field string) string {
	return fmt.Sprintf("alertsource:%s:%s", sourceID, field)
}

// ValidatedConfig is ValidateConfig's result: the non-secret keys destined
// for alert_sources.config_json, and the Secret-declared keys and their
// plaintext values destined for the secret store. Both maps are always
// non-nil (constructed with make, even when empty) because a nil map
// marshals to JSON null, which config_json's NOT NULL DEFAULT '{}' column
// cannot hold.
type ValidatedConfig struct {
	Public  map[string]any
	Secrets map[string]string
}

// ValidateConfig type-checks raw against spec, applies requiredness (relaxed
// for Secret fields under ConfigUpdate per D6), and resolves declared
// defaults. It is pure: no I/O, used at source create/update. raw is the API
// request body and still carries any plaintext secrets.
func ValidateConfig(spec *FieldSpec, raw map[string]any, mode ConfigMode) (*ValidatedConfig, error) {
	if err := checkConfigCall(spec, mode); err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	result := &ValidatedConfig{Public: map[string]any{}, Secrets: map[string]string{}}
	var errs []FieldError
	for _, e := range spec.fields {
		errs = append(errs, applyField(e, raw, mode, result)...)
	}
	if err := newValidationError(errs); err != nil {
		return nil, err
	}
	// applyField stores each field's CANONICAL Go value (shared with
	// LoadConfig's phase 1, which needs that same canonical form for
	// Config's accessors). Public alone is destined for config_json, so
	// only here is it rendered through D4's per-kind render — a raw
	// time.Duration would otherwise marshal as a JSON number and corrupt
	// the "Duration is a JSON string" contract on the next read.
	renderPublicForJSON(spec, result.Public)
	return result, nil
}

// renderPublicForJSON rewrites public's values in place through render, so
// every value is in the form D9 promises config_json holds. render is the
// identity for String/Int/Bool, so this only changes Duration entries.
func renderPublicForJSON(spec *FieldSpec, public map[string]any) {
	for _, e := range spec.fields {
		if e.secret {
			continue
		}
		if v, ok := public[e.name]; ok {
			public[e.name] = render(e.kind, v)
		}
	}
}

// checkConfigCall validates ValidateConfig's own call-level arguments
// (A2/D10: argument checks before spec validation, which runs before field
// processing).
func checkConfigCall(spec *FieldSpec, mode ConfigMode) error {
	if spec == nil {
		return newSpecError([]FieldError{{Field: "", Code: ErrCodeNilSpec, Message: "field spec must not be nil"}})
	}
	if mode != ConfigCreate && mode != ConfigUpdate {
		return newSpecError([]FieldError{{Field: "mode", Code: ErrCodeInvalidConfigMode, Message: "config mode must be ConfigCreate or ConfigUpdate"}})
	}
	return nil
}

// applyField type-checks, defaults and routes a single declared field from
// raw into result.Public or result.Secrets, per D6/D10/D15. It always
// stores each field's CANONICAL Go value (string/int/bool/time.Duration) —
// never the JSON-rendered form — because this routine is shared with
// LoadConfig's phase 1 (loadNonSecretFields), which needs that canonical
// value directly for Config's accessors. ValidateConfig alone renders
// Duration entries back to a JSON-safe string, afterward, via
// renderPublicForJSON.
func applyField(e fieldEntry, raw map[string]any, mode ConfigMode, result *ValidatedConfig) []FieldError {
	v, present := raw[e.name]

	if e.secret {
		return applySecretField(e, v, present, mode, result)
	}

	if !present {
		if e.hasDefault {
			result.Public[e.name] = e.defaultVal
			return nil
		}
		if !e.optional {
			return []FieldError{{Field: e.name, Code: ErrCodeMissingRequired, Message: msgFieldRequired}}
		}
		return nil
	}

	coerced, ok := coerceValue(e.kind, v)
	if !ok {
		// D10: a Duration value of the correct Go shape (a string) but
		// unparseable content is InvalidValue, not WrongType — coerceValue
		// folds both failure modes into ok=false, so the distinction is
		// made here from the ORIGINAL v, not from anything coerceValue
		// returns.
		if e.kind == FieldKindDuration {
			if _, isString := v.(string); isString {
				return []FieldError{{Field: e.name, Code: ErrCodeInvalidValue, Message: "not a valid Go duration"}}
			}
		}
		return []FieldError{{Field: e.name, Code: ErrCodeWrongType, Message: fmt.Sprintf("expected %s", e.kind.tag())}}
	}
	result.Public[e.name] = coerced
	return nil
}

// applySecretField handles D6/D15's Secret-specific requiredness relaxation
// and the "empty string is always rejected" rule.
func applySecretField(e fieldEntry, v any, present bool, mode ConfigMode, result *ValidatedConfig) []FieldError {
	if !present {
		if !e.optional && mode == ConfigCreate {
			return []FieldError{{Field: e.name, Code: ErrCodeMissingRequired, Message: msgFieldRequired}}
		}
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return []FieldError{{Field: e.name, Code: ErrCodeWrongType, Message: "expected string"}}
	}
	if s == "" {
		if !e.optional {
			return []FieldError{{Field: e.name, Code: ErrCodeMissingRequired, Message: msgFieldRequired}}
		}
		return []FieldError{{Field: e.name, Code: ErrCodeInvalidValue, Message: "an empty secret value is not accepted; omit the field to leave it unchanged"}}
	}
	result.Secrets[e.name] = s
	return nil
}

// Config is what Enrich and Poll receive: a wrapper whose only exported
// surface is four typed accessors, mirroring Benthos's ParsedConfig. Only
// LoadConfig constructs one, so an unvalidated map can never reach a
// source.
type Config struct {
	values map[string]any
}

// GetString returns name's value and whether it is present.
func (c Config) GetString(name string) (string, bool) {
	v, ok := c.values[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetInt returns name's value and whether it is present.
func (c Config) GetInt(name string) (int, bool) {
	v, ok := c.values[name]
	if !ok {
		return 0, false
	}
	n, ok := v.(int)
	return n, ok
}

// GetBool returns name's value and whether it is present.
func (c Config) GetBool(name string) (bool, bool) {
	v, ok := c.values[name]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetDuration returns name's value and whether it is present.
func (c Config) GetDuration(name string) (time.Duration, bool) {
	v, ok := c.values[name]
	if !ok {
		return 0, false
	}
	d, ok := v.(time.Duration)
	return d, ok
}

// String returns a redacted rendering: declared field names with every
// value replaced by a fixed marker. fmt reaches unexported struct fields by
// reflection, so an exported accessor-only surface does not by itself make
// %+v safe — String/GoString are what make it safe.
func (c Config) String() string { return c.redacted() }

// GoString mirrors String: %#v dispatches to GoStringer, and Go's default
// %#v rendering of an unexported map would otherwise print every value.
func (c Config) GoString() string { return c.redacted() }

func (c Config) redacted() string {
	redacted := make(map[string]string, len(c.values))
	for k := range c.values {
		redacted[k] = secretRedactionMarker
	}
	return fmt.Sprintf("%v", redacted)
}

// LoadConfig resolves a stored (secret-stripped) config plus the live secret
// store into a Config, for use at ingest/poll time. sourceID is required
// because SecretKey needs it and stored does not carry one.
func LoadConfig(ctx context.Context, spec *FieldSpec, sourceID string, stored map[string]any, r SecretResolver) (Config, error) {
	if err := checkLoadCall(spec, sourceID, r); err != nil {
		return Config{}, err
	}
	if err := spec.Validate(); err != nil {
		return Config{}, err
	}

	values, errs := loadNonSecretFields(spec, stored)
	if err := newValidationError(errs); err != nil {
		return Config{}, err
	}

	secretValues, err := loadSecretFields(ctx, spec, sourceID, r)
	if err != nil {
		return Config{}, err
	}
	if err := newValidationError(secretValues.errs); err != nil {
		return Config{}, err
	}
	for k, v := range secretValues.values {
		values[k] = v
	}
	return Config{values: values}, nil
}

// checkLoadCall validates LoadConfig's own call-level arguments, in the
// order A2 pins: NilSpec before EmptySourceID before NilResolver, none of
// which requires spec.Validate() to have run.
func checkLoadCall(spec *FieldSpec, sourceID string, r SecretResolver) error {
	if spec == nil {
		return newSpecError([]FieldError{{Field: "", Code: ErrCodeNilSpec, Message: "field spec must not be nil"}})
	}
	if sourceID == "" {
		return newSpecError([]FieldError{{Field: "sourceID", Code: ErrCodeEmptySourceID, Message: "source id must not be empty"}})
	}
	if isNilInterfaceValue(r) && specDeclaresSecret(spec) {
		return newSpecError([]FieldError{{Field: "", Code: ErrCodeNilResolver, Message: "a secret resolver is required because the spec declares a Secret field"}})
	}
	return nil
}

// specDeclaresSecret reports whether spec declares at least one Secret
// field. isNil entries are skipped: Validate() has already run by every
// call site that needs this answered, so a nil entry never reaches here in
// practice, but this helper does not assume that ordering itself.
func specDeclaresSecret(spec *FieldSpec) bool {
	for _, e := range spec.fields {
		if !e.isNil && e.secret {
			return true
		}
	}
	return false
}

// loadNonSecretFields is LoadConfig's pure phase 1: type-check and default
// every non-secret field, reusing applyField's per-field routine via a
// throwaway ValidatedConfig so the rule exists in exactly one place. Errors
// here short-circuit phase 2 (the resolver is never called for a config
// already known to be invalid).
func loadNonSecretFields(spec *FieldSpec, stored map[string]any) (map[string]any, []FieldError) {
	values := map[string]any{}
	var errs []FieldError
	scratch := &ValidatedConfig{Public: map[string]any{}, Secrets: map[string]string{}}
	for _, e := range spec.fields {
		if e.secret {
			continue
		}
		fieldErrs := applyField(e, stored, ConfigCreate, scratch)
		errs = append(errs, fieldErrs...)
	}
	for k, v := range scratch.Public {
		values[k] = v
	}
	return values, errs
}

// secretLoadResult separates a resolved secret value from a validation
// error so loadSecretFields can distinguish "no value" from "outage" for
// its caller without a third return value.
type secretLoadResult struct {
	values map[string]any
	errs   []FieldError
}

// loadSecretFields is LoadConfig's phase 2: resolve every declared Secret
// field, in declaration order, ALWAYS calling Reveal exactly once per field
// regardless of what earlier fields resolved to — an outage on field 1 must
// not suppress the resolver call for field 2. Only after every field has
// been resolved does an outage's classification win: if any non-ErrNotFound
// error occurred, that error (the first one encountered, by construction of
// the loop) is returned wrapped and every collected absence is discarded —
// a failing store's not-found answers are not trustworthy.
func loadSecretFields(ctx context.Context, spec *FieldSpec, sourceID string, r SecretResolver) (secretLoadResult, error) {
	result := secretLoadResult{values: map[string]any{}}
	var outageErr error
	for _, e := range spec.fields {
		if !e.secret {
			continue
		}
		value, err := r.Reveal(ctx, SecretKey(sourceID, e.name))
		if err != nil && !errors.Is(err, secrets.ErrNotFound) {
			if outageErr == nil {
				outageErr = fmt.Errorf("resolving secret %q: %w", e.name, err)
			}
			continue
		}
		absent := errors.Is(err, secrets.ErrNotFound) || value == ""
		if absent {
			if !e.optional {
				result.errs = append(result.errs, FieldError{Field: e.name, Code: ErrCodeMissingRequired, Message: msgFieldRequired})
			}
			continue
		}
		result.values[e.name] = value
	}
	if outageErr != nil {
		return secretLoadResult{}, outageErr
	}
	return result, nil
}
