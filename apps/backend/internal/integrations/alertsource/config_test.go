package alertsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/secrets"
)

func basicSpec() *FieldSpec {
	return NewFieldSpec().
		Field(NewStringField("site")).
		Field(NewIntField("poll_interval").Default(30)).
		Field(NewBoolField("verify_tls").Optional()).
		Field(NewDurationField("timeout").Optional()).
		Field(NewStringField("api_token").Secret())
}

// --- ValidateConfig ---

func TestValidateConfig_NilSpec(t *testing.T) {
	_, err := ValidateConfig(nil, map[string]any{}, ConfigCreate)
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeNilSpec)
}

func TestValidateConfig_InvalidMode(t *testing.T) {
	_, err := ValidateConfig(basicSpec(), map[string]any{}, ConfigMode(99))
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeInvalidConfigMode)
}

func TestValidateConfig_ZeroValueModeIsCreate(t *testing.T) {
	var mode ConfigMode
	if mode != ConfigCreate {
		t.Fatalf("ConfigMode zero value must be ConfigCreate, got %v", mode)
	}
}

func TestValidateConfig_InvalidSpecReportsSpecError(t *testing.T) {
	spec := NewFieldSpec().Field(nil)
	_, err := ValidateConfig(spec, map[string]any{}, ConfigCreate)
	_ = assertSpecError(t, err)
}

func TestValidateConfig_MissingRequiredNonSecret(t *testing.T) {
	raw := map[string]any{"api_token": "secret-value"}
	_, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "site", ErrCodeMissingRequired)
}

func TestValidateConfig_MissingRequiredSecret_UnderCreate(t *testing.T) {
	raw := map[string]any{"site": "datadoghq.com"}
	_, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "api_token", ErrCodeMissingRequired)
}

func TestValidateConfig_MissingSecret_LegalUnderUpdate(t *testing.T) {
	raw := map[string]any{"site": "datadoghq.com"}
	vc, err := ValidateConfig(basicSpec(), raw, ConfigUpdate)
	if err != nil {
		t.Fatalf("expected no error under ConfigUpdate, got %v", err)
	}
	if _, ok := vc.Secrets["api_token"]; ok {
		t.Fatalf("expected api_token to be absent from Secrets, got %v", vc.Secrets)
	}
}

func TestValidateConfig_MissingRequiredNonSecret_SameUnderBothModes(t *testing.T) {
	raw := map[string]any{"api_token": "secret-value"}
	for _, mode := range []ConfigMode{ConfigCreate, ConfigUpdate} {
		_, err := ValidateConfig(basicSpec(), raw, mode)
		verr := assertValidationError(t, err)
		assertValidationHasCode(t, verr, "site", ErrCodeMissingRequired)
	}
}

func TestValidateConfig_EmptySecretRejectedBothModes(t *testing.T) {
	for _, mode := range []ConfigMode{ConfigCreate, ConfigUpdate} {
		raw := map[string]any{"site": "datadoghq.com", "api_token": ""}
		_, err := ValidateConfig(basicSpec(), raw, mode)
		verr := assertValidationError(t, err)
		found := false
		for _, e := range verr.Errors {
			if e.Field == "api_token" {
				found = true
			}
		}
		if !found {
			t.Fatalf("mode %v: expected an error for empty secret, got %+v", mode, verr.Errors)
		}
	}
}

func TestValidateConfig_DefaultsApplied(t *testing.T) {
	raw := map[string]any{"site": "datadoghq.com", "api_token": "secret-value"}
	vc, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vc.Public["poll_interval"] != 30 {
		t.Fatalf("expected default poll_interval 30, got %v", vc.Public["poll_interval"])
	}
}

func TestValidateConfig_WrongType(t *testing.T) {
	raw := map[string]any{"site": 123, "api_token": "secret-value"}
	_, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "site", ErrCodeWrongType)
}

func TestValidateConfig_IntAcceptsJSONNumber(t *testing.T) {
	raw := map[string]any{"site": "x", "poll_interval": float64(60), "api_token": "s"}
	vc, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vc.Public["poll_interval"] != 60 {
		t.Fatalf("expected poll_interval 60, got %v (%T)", vc.Public["poll_interval"], vc.Public["poll_interval"])
	}
}

func TestValidateConfig_IntRejectsNonIntegralFloat(t *testing.T) {
	raw := map[string]any{"site": "x", "poll_interval": 1.5, "api_token": "s"}
	_, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "poll_interval", ErrCodeWrongType)
}

func TestValidateConfig_DurationInvalidValueVsWrongType(t *testing.T) {
	raw := map[string]any{"site": "x", "api_token": "s", "timeout": "5 minutes"}
	_, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "timeout", ErrCodeInvalidValue)

	raw2 := map[string]any{"site": "x", "api_token": "s", "timeout": 5}
	_, err2 := ValidateConfig(basicSpec(), raw2, ConfigCreate)
	verr2 := assertValidationError(t, err2)
	assertValidationHasCode(t, verr2, "timeout", ErrCodeWrongType)
}

func TestValidateConfig_SecretNeverInPublic(t *testing.T) {
	const sentinel = "sentinel-plaintext-value"
	raw := map[string]any{"site": "x", "api_token": sentinel}
	vc, err := ValidateConfig(basicSpec(), raw, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := vc.Public["api_token"]; ok {
		t.Fatalf("api_token must never appear in Public: %v", vc.Public)
	}
	b, _ := json.Marshal(vc.Public)
	if strings.Contains(string(b), sentinel) {
		t.Fatalf("sentinel leaked into marshalled Public: %s", b)
	}
	if vc.Secrets["api_token"] != sentinel {
		t.Fatalf("expected sentinel in Secrets, got %v", vc.Secrets)
	}
}

func TestValidateConfig_PublicAndSecretsAlwaysNonNilAndMarshalEmptyObject(t *testing.T) {
	spec := NewFieldSpec()
	vc, err := ValidateConfig(spec, map[string]any{}, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vc.Public == nil || vc.Secrets == nil {
		t.Fatalf("Public and Secrets must be non-nil, got %#v", vc)
	}
	b, err := json.Marshal(vc.Public)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("expected {} for an empty spec's Public, got %s", b)
	}
}

// TestValidateConfig_DurationRendersAsJSONStringInPublic guards the exact
// defect this layering was designed to avoid: config_json's contract is
// that a Duration is stored as a Go-syntax STRING, and encoding/json's
// default marshalling of a raw time.Duration produces a JSON NUMBER
// (nanoseconds) instead, which would corrupt every round trip through
// config_json.
func TestValidateConfig_DurationRendersAsJSONStringInPublic(t *testing.T) {
	spec := NewFieldSpec().Field(NewDurationField("timeout").Default(5 * time.Second))
	vc, err := ValidateConfig(spec, map[string]any{}, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vc.Public["timeout"] != "5s" {
		t.Fatalf("expected Public[timeout] to be the rendered string \"5s\", got %#v", vc.Public["timeout"])
	}
	b, err := json.Marshal(vc.Public)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !strings.Contains(string(b), `"5s"`) {
		t.Fatalf("expected marshalled Public to hold the duration as a JSON string, got %s", b)
	}
}

func TestValidateConfig_NilRawIsEmpty(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("x").Optional())
	vc, err := ValidateConfig(spec, nil, ConfigCreate)
	if err != nil {
		t.Fatalf("unexpected error for nil raw with all-optional spec: %v", err)
	}
	if len(vc.Public) != 0 {
		t.Fatalf("expected empty Public, got %v", vc.Public)
	}
}

// --- LoadConfig ---

type fakeResolver struct {
	values map[string]string
	errs   map[string]error
	calls  []string
}

func (f *fakeResolver) Reveal(_ context.Context, id string) (string, error) {
	f.calls = append(f.calls, id)
	if err, ok := f.errs[id]; ok {
		return "", err
	}
	if v, ok := f.values[id]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}

func TestLoadConfig_NilSpec(t *testing.T) {
	_, err := LoadConfig(context.Background(), nil, "src-1", map[string]any{}, &fakeResolver{})
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeNilSpec)
}

func TestLoadConfig_EmptySourceID(t *testing.T) {
	_, err := LoadConfig(context.Background(), basicSpec(), "", map[string]any{}, &fakeResolver{})
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeEmptySourceID)
}

// TestLoadConfig_A2_ArgumentChecksOutrankSpecValidation is A2's pinned
// ordering: an invalid spec combined with a bad argument reports the
// argument code ONLY, and the spec's own errors surface once the argument
// is fixed.
func TestLoadConfig_A2_ArgumentChecksOutrankSpecValidation(t *testing.T) {
	badSpec := NewFieldSpec().Field(nil)

	_, err := LoadConfig(context.Background(), badSpec, "", map[string]any{}, &fakeResolver{})
	serr := assertSpecError(t, err)
	if len(serr.Errors) != 1 || serr.Errors[0].Code != ErrCodeEmptySourceID {
		t.Fatalf("expected exactly the EmptySourceID argument error, got %+v", serr.Errors)
	}

	_, err2 := LoadConfig(context.Background(), badSpec, "src-1", map[string]any{}, &fakeResolver{})
	serr2 := assertSpecError(t, err2)
	assertHasCode(t, serr2, ErrCodeNilField)
}

func TestLoadConfig_NilResolver_LegalWithNoSecretFields(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("site"))
	_, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{"site": "x"}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLoadConfig_NilResolver_ErrorWhenSpecDeclaresSecret(t *testing.T) {
	_, err := LoadConfig(context.Background(), basicSpec(), "src-1", map[string]any{"site": "x"}, nil)
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeNilResolver)
}

// TestLoadConfig_TypedNilResolver_ErrorWhenSpecDeclaresSecret is R5-09: a
// typed nil (*fakeResolver)(nil) is non-nil under r == nil and would panic
// inside Reveal instead of reporting NilResolver, the same hazard D18
// already names for Source (isNilInterfaceValue covers both).
func TestLoadConfig_TypedNilResolver_ErrorWhenSpecDeclaresSecret(t *testing.T) {
	var r *fakeResolver
	_, err := LoadConfig(context.Background(), basicSpec(), "src-1", map[string]any{"site": "x"}, r)
	serr := assertSpecError(t, err)
	assertHasCode(t, serr, ErrCodeNilResolver)
}

func TestLoadConfig_PureFailureNeverCallsResolver(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{}}
	_, err := LoadConfig(context.Background(), basicSpec(), "src-1", map[string]any{}, resolver)
	_ = assertValidationError(t, err)
	if len(resolver.calls) != 0 {
		t.Fatalf("expected zero resolver calls on pure-phase failure, got %v", resolver.calls)
	}
}

func TestLoadConfig_ResolvesSecret(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): "the-secret",
	}}
	stored := map[string]any{"site": "datadoghq.com"}
	cfg, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := cfg.GetString("api_token"); !ok || got != "the-secret" {
		t.Fatalf("expected resolved secret, got (%q, %v)", got, ok)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != SecretKey("src-1", "api_token") {
		t.Fatalf("unexpected resolver calls: %v", resolver.calls)
	}
}

// TestConfig_String_RedactsEveryDeclaredFieldByName is R5-12: the mandated
// shape is "declared field names with values replaced", not merely "the
// real secret value is absent" — a constant "redacted" string would pass a
// sentinel-absence-only check while losing every field name. Map key order
// is deterministic here because fmt sorts string map keys when formatting.
func TestConfig_String_RedactsEveryDeclaredFieldByName(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): "the-secret",
	}}
	stored := map[string]any{"site": "datadoghq.com", "poll_interval": 60}
	cfg, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := cfg.String()
	if strings.Contains(got, "the-secret") {
		t.Fatalf("String() leaked the real secret value: %s", got)
	}
	want := fmt.Sprintf("map[api_token:%s poll_interval:%s site:%s]",
		secretRedactionMarker, secretRedactionMarker, secretRedactionMarker)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if cfg.GoString() != got {
		t.Fatalf("GoString() must match String(): %q vs %q", cfg.GoString(), got)
	}
}

func TestLoadConfig_AbsentSecret_RequiredYieldsMissingRequired(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{}}
	stored := map[string]any{"site": "datadoghq.com"}
	_, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	verr := assertValidationError(t, err)
	assertValidationHasCode(t, verr, "api_token", ErrCodeMissingRequired)
}

func TestLoadConfig_AbsentSecret_OptionalIsSimplyUnset(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("site")).
		Field(NewStringField("api_token").Secret().Optional())
	resolver := &fakeResolver{values: map[string]string{}}
	cfg, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{"site": "x"}, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.GetString("api_token"); ok {
		t.Fatalf("expected api_token to be absent")
	}
}

func TestLoadConfig_RevealEmptyStringCountsAsAbsent(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("site")).
		Field(NewStringField("api_token").Secret().Optional())
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): "",
	}}
	cfg, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{"site": "x"}, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.GetString("api_token"); ok {
		t.Fatalf("expected empty-string reveal to count as absent")
	}
}

func TestLoadConfig_Outage_WinsOverAbsence_AndSkipsValidationError(t *testing.T) {
	outageErr := errors.New("store outage")
	spec := NewFieldSpec().
		Field(NewStringField("token_a").Secret().Optional()).
		Field(NewStringField("token_b").Secret().Optional())
	resolver := &fakeResolver{
		values: map[string]string{},
		errs:   map[string]error{SecretKey("src-1", "token_a"): outageErr},
	}
	_, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{}, resolver)
	if err == nil {
		t.Fatal("expected an outage error")
	}
	if _, ok := err.(*ValidationError); ok {
		t.Fatalf("outage must not surface as a ValidationError, got %#v", err)
	}
	if !errors.Is(err, outageErr) {
		t.Fatalf("expected wrapped outage error, got %v", err)
	}
}

func TestLoadConfig_Outage_CallsResolverForEveryDeclaredSecretField(t *testing.T) {
	outageErr := errors.New("store outage")
	spec := NewFieldSpec().
		Field(NewStringField("token_a").Secret().Optional()).
		Field(NewStringField("token_b").Secret().Optional())
	resolver := &fakeResolver{
		values: map[string]string{},
		errs:   map[string]error{SecretKey("src-1", "token_a"): outageErr},
	}
	if _, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{}, resolver); err == nil {
		t.Fatal("expected an outage error")
	}
	if len(resolver.calls) != 2 {
		t.Fatalf("expected one resolver call per declared secret field even after an outage, got %v", resolver.calls)
	}
}

func TestLoadConfig_MixedAbsentAndOutage_AssertsOutage(t *testing.T) {
	outageErr := errors.New("store outage")
	spec := NewFieldSpec().
		Field(NewStringField("token_a").Secret().Optional()).
		Field(NewStringField("token_b").Secret().Optional())
	resolver := &fakeResolver{
		values: map[string]string{},
		errs:   map[string]error{SecretKey("src-1", "token_b"): outageErr},
	}
	_, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{}, resolver)
	if !errors.Is(err, outageErr) {
		t.Fatalf("expected the outage error to win over the absence, got %v", err)
	}
}

func TestLoadConfig_StraySecretInStoredIsIgnored(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): "the-real-secret",
	}}
	stored := map[string]any{"site": "datadoghq.com", "api_token": "stale-plaintext"}
	cfg, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := cfg.GetString("api_token")
	if got != "the-real-secret" {
		t.Fatalf("expected the resolver's value, got %q (stray stored value must never be read)", got)
	}
}

func TestLoadConfig_NilStoredIsEmpty(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("x").Optional())
	cfg, err := LoadConfig(context.Background(), spec, "src-1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.GetString("x"); ok {
		t.Fatalf("expected x to be absent")
	}
}

// --- Config accessors ---

func TestConfig_Accessors_PresenceAndKindIsolation(t *testing.T) {
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): "the-secret",
	}}
	stored := map[string]any{"site": "datadoghq.com", "verify_tls": true, "timeout": "30s"}
	cfg, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s, ok := cfg.GetString("site"); !ok || s != "datadoghq.com" {
		t.Fatalf("GetString(site) = (%q, %v)", s, ok)
	}
	if n, ok := cfg.GetInt("poll_interval"); !ok || n != 30 {
		t.Fatalf("GetInt(poll_interval) = (%v, %v), want default 30", n, ok)
	}
	if b, ok := cfg.GetBool("verify_tls"); !ok || !b {
		t.Fatalf("GetBool(verify_tls) = (%v, %v)", b, ok)
	}
	if d, ok := cfg.GetDuration("timeout"); !ok || d != 30*time.Second {
		t.Fatalf("GetDuration(timeout) = (%v, %v)", d, ok)
	}
	if s, ok := cfg.GetString("api_token"); !ok || s != "the-secret" {
		t.Fatalf("GetString(api_token) = (%q, %v)", s, ok)
	}

	// Kind mismatch on an undeclared name never panics; returns zero, false.
	if _, ok := cfg.GetInt("site"); ok {
		t.Fatal("expected GetInt(site) to report absent for a string-valued name")
	}
	if _, ok := cfg.GetString("does_not_exist"); ok {
		t.Fatal("expected GetString on an undeclared name to report absent")
	}
}

func TestConfig_Accessors_CaseSensitive(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("site"))
	cfg, err := LoadConfig(context.Background(), spec, "src-1", map[string]any{"site": "x"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.GetString("Site"); ok {
		t.Fatal("expected case-sensitive lookup to miss on differing case")
	}
}

// TestConfig_RedactionCoversAllThreeVerbs is D6's mandated test: %v, %+v and
// %#v must never leak a value, because fmt reaches unexported fields by
// reflection regardless of the exported accessor surface.
func TestConfig_RedactionCoversAllThreeVerbs(t *testing.T) {
	const sentinel = "the-plaintext-secret-value"
	resolver := &fakeResolver{values: map[string]string{
		SecretKey("src-1", "api_token"): sentinel,
	}}
	stored := map[string]any{"site": "datadoghq.com"}
	cfg, err := LoadConfig(context.Background(), basicSpec(), "src-1", stored, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	forms := []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
	}
	for _, s := range forms {
		if strings.Contains(s, sentinel) {
			t.Fatalf("sentinel leaked through a fmt verb: %s", s)
		}
	}
}

// --- SecretKey ---

func TestSecretKey_Format(t *testing.T) {
	got := SecretKey("src-1", "api_token")
	want := "alertsource:src-1:api_token"
	if got != want {
		t.Fatalf("SecretKey = %q, want %q", got, want)
	}
}

// --- test helpers ---

func assertValidationHasCode(t *testing.T, verr *ValidationError, field string, code ErrorCode) {
	t.Helper()
	for _, e := range verr.Errors {
		if e.Field == field && e.Code == code {
			return
		}
	}
	t.Fatalf("expected field %q to carry code %q, got %+v", field, code, verr.Errors)
}
