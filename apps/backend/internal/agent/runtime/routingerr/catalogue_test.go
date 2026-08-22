package routingerr

import (
	"net/http"
	"reflect"
	"testing"
)

func TestClassForCodeCoversProviderCatalogue(t *testing.T) {
	tests := []struct {
		code Code
		want Class
	}{
		{CodeNetworkUnavailable, ClassTransient},
		{CodeProviderUnavailable, ClassTransient},
		{CodeProviderOverloaded, ClassTransient},
		{CodeModelCapacity, ClassTransient},
		{CodeRateLimited, ClassTransient},
		{CodeAgentTransportLost, ClassTransient},
		{CodeAuthRequired, ClassHard},
		{CodeMissingCredentials, ClassHard},
		{CodeSubscriptionRequired, ClassHard},
		{CodeQuotaLimited, ClassHard},
		{CodeModelUnavailable, ClassHard},
		{CodeProviderNotConfigured, ClassHard},
		{CodeUnknownProvider, ClassUnclassified},
		{CodeAgentRuntime, ClassUnclassified},
		{CodeTask, ClassUnclassified},
		{CodeRepo, ClassUnclassified},
		{CodePermissionDeniedByUser, ClassUnclassified},
		{CodeNpxCacheCorrupted, ClassUnclassified},
		{CodeManagedRuntimeNpmResolution, ClassUnclassified},
		{CodeResumeCorrupted, ClassUnclassified},
	}

	for _, test := range tests {
		if got := ClassForCode(test.code); got != test.want {
			t.Errorf("ClassForCode(%q) = %q, want %q", test.code, got, test.want)
		}
	}
	if got := ClassForCode(Code("future_provider_code")); got != ClassUnclassified {
		t.Fatalf("unknown code class = %q, want %q", got, ClassUnclassified)
	}
}

func TestClassifyPersistsCatalogueVersion(t *testing.T) {
	classified := Classify(Input{Phase: PhasePromptSend, Stderr: "temporary failure"})
	if classified.CatalogueVersion != CatalogueVersion {
		t.Fatalf("catalogue version = %q, want %q", classified.CatalogueVersion, CatalogueVersion)
	}
}

func classifiedErrorClass(e *Error) string {
	if e == nil {
		return ""
	}
	value := reflect.ValueOf(e).Elem().FieldByName("Class")
	if !value.IsValid() || value.Kind() != reflect.String {
		return ""
	}
	return value.String()
}

func TestClassifyAssignsSharedProviderErrorClasses(t *testing.T) {
	tests := []struct {
		name  string
		input Input
		want  string
	}{
		{
			name: "transient provider overload",
			input: Input{
				Phase:      PhasePromptSend,
				HTTPStatus: statusOverloaded,
			},
			want: "transient",
		},
		{
			name: "transient rate limit",
			input: Input{
				Phase:      PhasePromptSend,
				HTTPStatus: http.StatusTooManyRequests,
			},
			want: "transient",
		},
		{
			name: "hard subscription",
			input: Input{
				Phase:      PhasePromptSend,
				HTTPStatus: http.StatusPaymentRequired,
			},
			want: "hard",
		},
		{
			name: "hard authentication",
			input: Input{
				Phase:      PhasePromptSend,
				HTTPStatus: http.StatusUnauthorized,
			},
			want: "hard",
		},
		{
			name: "unknown fails closed",
			input: Input{
				Phase:  PhasePromptSend,
				Stderr: "provider returned an unfamiliar failure",
			},
			want: "unclassified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified := Classify(test.input)
			if got := classifiedErrorClass(classified); got != test.want {
				t.Fatalf("Classify(...).Class = %q, want %q", got, test.want)
			}
		})
	}
}
