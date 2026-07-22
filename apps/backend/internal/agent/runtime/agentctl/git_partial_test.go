package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitCreatePRDecodesBranchPushedPartialState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"branch_pushed":true,"provider":"gitlab","error":"branch was pushed; retry merge request creation"}`))
	}))
	t.Cleanup(server.Close)
	client := &Client{baseURL: server.URL, httpClient: server.Client()}

	result, err := client.GitCreatePR(context.Background(), "Title", "Body", "main", false, "")
	if err != nil {
		t.Fatalf("GitCreatePR: %v", err)
	}
	if result.Success || !result.BranchPushed || result.Provider != "gitlab" ||
		result.Error != "branch was pushed; retry merge request creation" {
		t.Fatalf("result = %+v", result)
	}
}
