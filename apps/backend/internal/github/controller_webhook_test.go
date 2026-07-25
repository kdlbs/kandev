package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTTPGitHubWebhookRoutesByAppRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &githubWebhookMemoryStore{}
	webhooks := NewAppRegistrationWebhookService(
		"registration-work", "work-secret", store, nil, nil,
	)
	runtime := &githubAppRuntime{
		registrationID: "registration-work", source: DeploymentAppSourceManaged,
		generation: 1, webhookAuth: webhooks,
	}
	service := &Service{appRegistrationRuntimes: map[string]*githubAppRuntime{
		"registration-work": runtime,
	}}
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)

	payload := `{"action":"created","installation":{"id":42}}`
	request := signedWebhookRequest(
		"work-secret", "delivery-1", "installation", []byte(payload),
	)
	response := performWebhookRequest(
		router, "/api/v1/github/app/registrations/registration-work/webhook", request,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("matching route status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("matching route deliveries = %#v", store.deliveries)
	}

	response = performWebhookRequest(
		router, "/api/v1/github/app/registrations/registration-personal/webhook", request,
	)
	if response.Code != http.StatusConflict || len(store.deliveries) != 1 {
		t.Fatalf(
			"wrong route status = %d, deliveries = %#v, body = %s",
			response.Code, store.deliveries, response.Body.String(),
		)
	}

	response = performWebhookRequest(router, "/api/v1/github/app/webhook", request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy route status = %d, want 404", response.Code)
	}
}

func performWebhookRequest(
	router http.Handler,
	path string,
	request GitHubWebhookRequest,
) *httptest.ResponseRecorder {
	httpRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(request.Payload)))
	httpRequest.Header.Set("X-GitHub-Delivery", request.DeliveryID)
	httpRequest.Header.Set("X-GitHub-Event", request.Event)
	httpRequest.Header.Set("X-Hub-Signature-256", request.Signature)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httpRequest)
	return response
}
