package nomad

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRegisterTreatsServerErrorAsUnknownOutcome(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		client, err := NewClient(server.URL, "fixture-token")
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.RegisterCreateOnly(t.Context(), Job{ID: "job-1"})
		server.Close()
		if !errors.Is(err, ErrRegistrationOutcomeUnknown) {
			t.Fatalf("HTTP %d: err = %v, want ErrRegistrationOutcomeUnknown", status, err)
		}
	}
}

func TestClientRegisterTreatsForbiddenAsDefiniteRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	client, _ := NewClient(server.URL, "fixture-token")
	if _, err := client.RegisterCreateOnly(t.Context(), Job{ID: "job-1"}); err == nil || errors.Is(err, ErrRegistrationOutcomeUnknown) {
		t.Fatalf("err = %v, want a definite rejection", err)
	}
}

func TestIsNotFoundMatchesOnlyAuthoritative404(t *testing.T) {
	for status, want := range map[int]bool{http.StatusNotFound: true, http.StatusForbidden: false, http.StatusInternalServerError: false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		client, _ := NewClient(server.URL, "fixture-token")
		_, err := client.GetJob(t.Context(), "default", "job-1")
		server.Close()
		if IsNotFound(err) != want {
			t.Fatalf("HTTP %d: IsNotFound = %v, want %v", status, IsNotFound(err), want)
		}
	}
}
