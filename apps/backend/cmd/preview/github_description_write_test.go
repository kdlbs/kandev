package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func githubJSONResponse(req *http.Request, status int, payload any) *http.Response {
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    req,
	}
}

func TestUpsertDescriptionSectionPreservesBodyChangedBeforePatch(t *testing.T) {
	// @covers AC-UI-PR-WALKTHROUGH-001.10
	originalBody := "Contributor content"
	newerBody := originalBody + "\n\nPreview bot content"
	currentBody := originalBody
	getCount := 0
	var patchedBody string

	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			getCount++
			if getCount == 2 {
				currentBody = newerBody
			}
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		case http.MethodPatch:
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				return nil, err
			}
			patchedBody = payload.Body
			currentBody = patchedBody
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		default:
			return githubJSONResponse(req, http.StatusNotFound, map[string]string{}), nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	err := upsertDescriptionSection(
		context.Background(),
		"token",
		"owner/repo",
		1,
		"### Preview\nURL: https://example.com",
	)
	if err != nil {
		t.Fatalf("upsertDescriptionSection() error = %v", err)
	}
	if !strings.Contains(patchedBody, "Preview bot content") {
		t.Fatalf("patched body lost newer content: %q", patchedBody)
	}
	if !strings.Contains(patchedBody, sectionStart) {
		t.Fatalf("patched body lost preview section: %q", patchedBody)
	}
	if getCount < 4 {
		t.Fatalf("expected fresh snapshot and readback, got %d GET requests", getCount)
	}
}

func TestUpsertDescriptionSectionRetriesAfterReadbackLosesUpdate(t *testing.T) {
	// @covers AC-UI-PR-WALKTHROUGH-001.10
	currentBody := "Contributor content"
	patchCount := 0
	getCount := 0

	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			getCount++
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		case http.MethodPatch:
			patchCount++
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				return nil, err
			}
			if patchCount == 1 {
				currentBody = "External writer won"
			} else {
				currentBody = payload.Body
			}
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		default:
			return githubJSONResponse(req, http.StatusNotFound, map[string]string{}), nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	err := upsertDescriptionSection(
		context.Background(),
		"token",
		"owner/repo",
		1,
		"### Preview\nURL: https://example.com",
	)
	if err != nil {
		t.Fatalf("upsertDescriptionSection() error = %v", err)
	}
	if patchCount != 2 {
		t.Fatalf("expected a retry after readback loss, got %d PATCH requests", patchCount)
	}
	if !strings.Contains(currentBody, sectionStart) {
		t.Fatalf("final body lost preview section: %q", currentBody)
	}
	if !strings.Contains(currentBody, "External writer won") {
		t.Fatalf("final body lost external content: %q", currentBody)
	}
	if getCount < 6 {
		t.Fatalf("expected readback and retry reads, got %d GET requests", getCount)
	}
}

func TestUpsertDescriptionSectionFailsAfterRepeatedBodyChanges(t *testing.T) {
	// @covers AC-UI-PR-WALKTHROUGH-001.10
	getCount := 0
	patchCount := 0

	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			getCount++
			return githubJSONResponse(req, http.StatusOK, map[string]string{
				"body": "Body version " + string(rune('0'+getCount)),
			}), nil
		case http.MethodPatch:
			patchCount++
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": "ignored"}), nil
		default:
			return githubJSONResponse(req, http.StatusNotFound, map[string]string{}), nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	err := upsertDescriptionSection(
		context.Background(),
		"token",
		"owner/repo",
		1,
		"### Preview\nURL: https://example.com",
	)
	if err == nil {
		t.Fatal("upsertDescriptionSection() error = nil, want bounded convergence failure")
	}
	if !strings.Contains(err.Error(), "did not converge") {
		t.Fatalf("error = %v, want convergence failure", err)
	}
	if patchCount != 0 {
		t.Fatalf("expected no stale PATCH requests, got %d", patchCount)
	}
}

func TestRemoveDescriptionSectionPreservesOtherOwnedContent(t *testing.T) {
	// @covers AC-UI-PR-WALKTHROUGH-001.10
	walkthrough := "<!-- kandev-pr-walkthrough-start -->\nwalkthrough\n<!-- kandev-pr-walkthrough-end -->"
	currentBody := walkthrough + "\n\n" + sectionStart + "\npreview\n" + sectionEnd
	patchCount := 0

	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		case http.MethodPatch:
			patchCount++
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				return nil, err
			}
			currentBody = payload.Body
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		default:
			return githubJSONResponse(req, http.StatusNotFound, map[string]string{}), nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if err := removeDescriptionSection(context.Background(), "token", "owner/repo", 1); err != nil {
		t.Fatalf("removeDescriptionSection() error = %v", err)
	}
	if patchCount != 1 {
		t.Fatalf("expected one PATCH request, got %d", patchCount)
	}
	if !strings.Contains(currentBody, walkthrough) {
		t.Fatalf("removeDescriptionSection() lost walkthrough content: %q", currentBody)
	}
	if strings.Contains(currentBody, sectionStart) || strings.Contains(currentBody, sectionEnd) {
		t.Fatalf("removeDescriptionSection() kept preview markers: %q", currentBody)
	}
}

func TestRemoveDescriptionSectionWithoutMarkerDoesNotPatch(t *testing.T) {
	// @covers AC-UI-PR-WALKTHROUGH-001.10
	patchCount := 0
	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPatch {
			patchCount++
		}
		return githubJSONResponse(req, http.StatusOK, map[string]string{"body": "Contributor content"}), nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if err := removeDescriptionSection(context.Background(), "token", "owner/repo", 1); err != nil {
		t.Fatalf("removeDescriptionSection() error = %v", err)
	}
	if patchCount != 0 {
		t.Fatalf("expected no PATCH request without a marker, got %d", patchCount)
	}
}

func TestRemoveDescriptionSectionWithOrphanEndMarkerDoesNotPatch(t *testing.T) {
	currentBody := "Contributor content\n\n" + sectionEnd
	patchCount := 0
	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
		case http.MethodPatch:
			patchCount++
		}
		return githubJSONResponse(req, http.StatusOK, map[string]string{"body": currentBody}), nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if err := removeDescriptionSection(context.Background(), "token", "owner/repo", 1); err != nil {
		t.Fatalf("removeDescriptionSection() error = %v", err)
	}
	if patchCount != 0 {
		t.Fatalf("expected no PATCH request for an orphan end marker, got %d", patchCount)
	}
}
