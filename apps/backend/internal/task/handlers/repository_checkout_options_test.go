package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRepositoryCheckoutOptionsRequestConversion(t *testing.T) {
	var input httpTaskRepositoryInput
	if err := json.Unmarshal([]byte(`{"remote_url":"https://github.com/acme/repo","checkout_options":{"version":1,"download_mode":"on_demand","sparse_directories":["app"]}}`), &input); err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	repos, ok := convertCreateTaskRepositories(c, []httpTaskRepositoryInput{input})
	if !ok {
		t.Fatal("request rejected")
	}
	result := convertToServiceRepos(repos)
	if len(result) != 1 || result[0].CheckoutOptions == nil {
		t.Fatal("request conversion lost checkout options")
	}
	if result[0].CheckoutOptions.SparseDirectories[0] != "app" {
		t.Fatal("request conversion changed folder selection")
	}
}

func TestCheckoutCapabilitiesHTTP(t *testing.T) {
	router, _ := newRepositoryHTTPTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/repository-checkout-capabilities", bytes.NewBufferString(`{"repository":{"remote_url":"https://github.com/acme/repo"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d: %s", response.Code, response.Body.String())
	}
	var capabilities struct {
		OnDemand bool   `json:"on_demand"`
		Sparse   bool   `json:"sparse"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &capabilities); err != nil {
		t.Fatal(err)
	}
	if capabilities.OnDemand || capabilities.Sparse || capabilities.Reason != "executor_required" {
		t.Fatalf("unsupported capabilities = %+v", capabilities)
	}
}
