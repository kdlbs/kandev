package pluginsdk

import (
	"errors"
	"net/http"
	"testing"
)

func TestCategorizeActionErrorPreservesCause(t *testing.T) {
	cause := errors.New("provider rejected request")
	err := CategorizeActionError(ActionErrorConflict, cause)
	if !errors.Is(err, cause) {
		t.Fatal("categorized action error lost its cause")
	}
	var categorized *ActionError
	if !errors.As(err, &categorized) || categorized.Code != ActionErrorConflict {
		t.Fatalf("categorized error = %#v", categorized)
	}
}

func TestActionErrorHTTPStatusIsStableAcrossPlugins(t *testing.T) {
	tests := map[ActionErrorCode]int{
		ActionErrorInvalidArgument:  http.StatusBadRequest,
		ActionErrorUnauthenticated:  http.StatusUnauthorized,
		ActionErrorPermissionDenied: http.StatusForbidden,
		ActionErrorNotFound:         http.StatusNotFound,
		ActionErrorConflict:         http.StatusConflict,
		ActionErrorRateLimited:      http.StatusTooManyRequests,
		ActionErrorUnavailable:      http.StatusServiceUnavailable,
		ActionErrorUpstream:         http.StatusBadGateway,
	}
	for code, want := range tests {
		if got := ActionErrorHTTPStatus(code); got != want {
			t.Fatalf("ActionErrorHTTPStatus(%q) = %d, want %d", code, got, want)
		}
	}
	if got := ActionErrorHTTPStatus(ActionErrorCode("future")); got != http.StatusBadGateway {
		t.Fatalf("unknown action code status = %d", got)
	}
}

func TestCategorizeActionErrorWithRetryPreservesStableMetadata(t *testing.T) {
	cause := errors.New("provider throttled request")
	err := CategorizeActionErrorWithRetry(ActionErrorRateLimited, "120", cause)
	var categorized *ActionError
	if !errors.As(err, &categorized) {
		t.Fatal("categorized error metadata is unavailable")
	}
	if categorized.Code != ActionErrorRateLimited || categorized.RetryAfter != "120" {
		t.Fatalf("categorized error = %#v", categorized)
	}
	if !errors.Is(err, cause) {
		t.Fatal("categorized retry error lost its cause")
	}
}
