package alertsource

import (
	"context"
	"testing"
	"time"
)

func validDescriptor() Descriptor {
	return Descriptor{
		Type:              "datadog",
		DisplayName:       "Datadog",
		Category:          "monitoring",
		WatchMetadataKey:  "datadog_watch_id",
		FingerprintFields: []string{"alertname", "instance"},
		Config:            NewFieldSpec().Field(NewStringField("api_key").Secret()),
		Capabilities:      Capabilities{Webhook: true},
	}
}

func TestDescriptor_Validate_ValidDescriptorHasNoErrors(t *testing.T) {
	d := validDescriptor()
	if err := d.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDescriptor_Validate_EmptyType(t *testing.T) {
	d := validDescriptor()
	d.Type = ""
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeEmptyType)
}

func TestDescriptor_Validate_MissingWatchMetadataKey(t *testing.T) {
	d := validDescriptor()
	d.WatchMetadataKey = ""
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeMissingWatchMetadataKey)
}

func TestDescriptor_Validate_InvalidWatchMetadataKey_DottedOrHyphenated(t *testing.T) {
	for _, key := range []string{"datadog.watch-id", "datadog-watch-id", "datadog.watch.id"} {
		d := validDescriptor()
		d.WatchMetadataKey = key
		err := assertSpecError(t, d.Validate())
		assertHasCode(t, err, ErrCodeInvalidWatchMetadataKey)
	}
}

func TestDescriptor_Validate_EmptyFingerprintFields(t *testing.T) {
	d := validDescriptor()
	d.FingerprintFields = nil
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeEmptyFingerprintFields)
}

func TestDescriptor_Validate_EmptyFingerprintField(t *testing.T) {
	d := validDescriptor()
	d.FingerprintFields = []string{"alertname", ""}
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeEmptyFingerprintField)
	for _, fe := range err.Errors {
		if fe.Code == ErrCodeEmptyFingerprintField && fe.Field != "FingerprintFields[1]" {
			t.Errorf("expected FingerprintFields[1], got %q", fe.Field)
		}
	}
}

func TestDescriptor_Validate_DuplicateFingerprintField_CardinalityAndIndex(t *testing.T) {
	d := validDescriptor()
	d.FingerprintFields = []string{"a", "a", "a"}
	err := assertSpecError(t, d.Validate())

	var dupes []FieldError
	for _, fe := range err.Errors {
		if fe.Code == ErrCodeDuplicateFingerprintField {
			dupes = append(dupes, fe)
		}
	}
	if len(dupes) != 2 {
		t.Fatalf("expected exactly 2 duplicate-fingerprint-field errors, got %d: %+v", len(dupes), dupes)
	}
	wantFields := map[string]bool{"FingerprintFields[1]": true, "FingerprintFields[2]": true}
	for _, fe := range dupes {
		if !wantFields[fe.Field] {
			t.Errorf("unexpected duplicate-field path %q", fe.Field)
		}
		delete(wantFields, fe.Field)
	}
	if len(wantFields) != 0 {
		t.Errorf("missing duplicate-field paths: %v", wantFields)
	}
}

func TestDescriptor_Validate_NilConfig(t *testing.T) {
	d := validDescriptor()
	d.Config = nil
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeNilConfig)
}

func TestDescriptor_Validate_ConfigErrorsMergedWithPrefix(t *testing.T) {
	d := validDescriptor()
	d.Config = NewFieldSpec().Field(NewStringField("Bad-Name"))
	err := assertSpecError(t, d.Validate())

	found := false
	for _, fe := range err.Errors {
		if fe.Code == ErrCodeInvalidFieldName {
			found = true
			if fe.Field != "Config.Bad-Name" {
				t.Errorf("expected Field %q, got %q", "Config.Bad-Name", fe.Field)
			}
		}
	}
	if !found {
		t.Fatalf("expected a merged InvalidFieldName error, got %+v", err.Errors)
	}
}

func TestDescriptor_Validate_MissingCapability_NoSourceInScope(t *testing.T) {
	d := validDescriptor()
	d.Capabilities = Capabilities{}
	err := assertSpecError(t, d.Validate())
	assertHasCode(t, err, ErrCodeMissingCapability)
}

func TestDescriptor_Validate_CapabilitiesPollOnlyIsValid(t *testing.T) {
	d := validDescriptor()
	d.Capabilities = Capabilities{Poll: true}
	if err := d.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDescriptor_Validate_CollectsAllErrors(t *testing.T) {
	d := Descriptor{}
	err := assertSpecError(t, d.Validate())
	wantCodes := []ErrorCode{
		ErrCodeEmptyType,
		ErrCodeMissingWatchMetadataKey,
		ErrCodeEmptyFingerprintFields,
		ErrCodeNilConfig,
		ErrCodeMissingCapability,
	}
	for _, code := range wantCodes {
		assertHasCode(t, err, code)
	}
}

// fakeSourceBase implements only Source. fakeSourceWithPoller and
// fakeSourceWithEnricher each additionally implement exactly one optional
// interface — separate concrete types, not one type with optional embedded
// fields, because Go method promotion is resolved by the embedded field's
// TYPE, not its value: a nil *fakePoller field would still make the outer
// struct satisfy Poller, defeating a "not implemented" test.
type fakeSourceBase struct{}

func (fakeSourceBase) Descriptor() Descriptor { return Descriptor{} }
func (fakeSourceBase) Normalize(_ context.Context, _ []byte) ([]Alert, error) {
	return nil, nil
}

type fakeSourceWithPoller struct{ fakeSourceBase }

func (fakeSourceWithPoller) Poll(_ context.Context, _ Config, _ time.Time) ([]Alert, error) {
	return nil, nil
}

type fakeSourceWithEnricher struct{ fakeSourceBase }

func (fakeSourceWithEnricher) Enrich(_ context.Context, _ Config, _ *Alert) error { return nil }

func TestCheckCapabilities_NilSource_Untyped(t *testing.T) {
	d := validDescriptor()
	err := assertSpecError(t, CheckCapabilities(d, nil))
	assertHasCode(t, err, ErrCodeNilSource)
}

func TestCheckCapabilities_NilSource_TypedNil(t *testing.T) {
	d := validDescriptor()
	var s *fakeSourceBase
	err := assertSpecError(t, CheckCapabilities(d, s))
	assertHasCode(t, err, ErrCodeNilSource)
}

func TestCheckCapabilities_PollDeclaredButNotImplemented(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Poll = true
	err := assertSpecError(t, CheckCapabilities(d, fakeSourceBase{}))
	assertHasCode(t, err, ErrCodeCapabilityNotImplemented)
}

func TestCheckCapabilities_PollImplementedButNotDeclared(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Poll = false
	err := assertSpecError(t, CheckCapabilities(d, fakeSourceWithPoller{}))
	assertHasCode(t, err, ErrCodeCapabilityNotDeclared)
}

func TestCheckCapabilities_PollDeclaredAndImplementedIsValid(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Poll = true
	if err := CheckCapabilities(d, fakeSourceWithPoller{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckCapabilities_EnrichDeclaredButNotImplemented(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Enrich = true
	err := assertSpecError(t, CheckCapabilities(d, fakeSourceBase{}))
	assertHasCode(t, err, ErrCodeCapabilityNotImplemented)
}

func TestCheckCapabilities_EnrichImplementedButNotDeclared(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Enrich = false
	err := assertSpecError(t, CheckCapabilities(d, fakeSourceWithEnricher{}))
	assertHasCode(t, err, ErrCodeCapabilityNotDeclared)
}

func TestCheckCapabilities_EnrichDeclaredAndImplementedIsValid(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Enrich = true
	if err := CheckCapabilities(d, fakeSourceWithEnricher{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckCapabilities_WebhookOnlyNoPollNoEnrichIsValid(t *testing.T) {
	d := validDescriptor()
	if err := CheckCapabilities(d, fakeSourceBase{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckCapabilities_CollectsBothPollAndEnrichMismatches(t *testing.T) {
	d := validDescriptor()
	d.Capabilities.Poll = true
	d.Capabilities.Enrich = true
	err := assertSpecError(t, CheckCapabilities(d, fakeSourceBase{}))
	assertHasCode(t, err, ErrCodeCapabilityNotImplemented)
	fields := map[string]int{}
	for _, fe := range err.Errors {
		if fe.Code == ErrCodeCapabilityNotImplemented {
			fields[fe.Field]++
		}
	}
	if len(fields) != 2 {
		t.Fatalf("expected 2 distinctly-identified CapabilityNotImplemented errors (Poll and Enrich), got fields %v: %+v", fields, err.Errors)
	}
	if fields["Capabilities.Poll"] != 1 || fields["Capabilities.Enrich"] != 1 {
		t.Fatalf("expected exactly one CapabilityNotImplemented error each for Capabilities.Poll and Capabilities.Enrich, got %v", fields)
	}
}
