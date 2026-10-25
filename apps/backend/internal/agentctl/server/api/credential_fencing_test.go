package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCredentialScopedRequest_ControlConsumption(t *testing.T) {
	srv := newTestServer(t)
	creds := newCredentialState("control-token")
	_, replacement, err := creds.Rotate("control-token")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		auth       gin.HandlerFunc
		header     string
		wantHeader bool
		wantStatus int
	}{
		{"legacy static", bearerTokenAuth("control-token"), "Bearer control-token", false, 200},
		{"instance static", srv.instanceAuth("control-token"), "Bearer control-token", false, 200},
		{"control rotating", controlCredentialAuth(creds, nil), "Bearer " + replacement, false, 200},
		{"adoption only", controlCredentialAuth(creds, map[string]bool{"/app": true}), "Bearer control-token", false, 200},
		{"disabled bearer", srv.instanceAuth(""), "Bearer app-token", true, 200},
		{"disabled basic", srv.instanceAuth(""), "Basic app-auth", true, 200},
		{"disabled legacy", bearerTokenAuth(""), "Bearer app-token", true, 200},
		{"disabled control", controlCredentialAuth(newCredentialState(""), nil), "Bearer app-token", true, 200},
		{"exempt instance", srv.instanceAuth("control-token", "/app"), "Bearer app-token", true, 200},
		{"exempt legacy", bearerTokenAuth("control-token", "/app"), "Bearer app-token", true, 200},
		{"exempt control", controlCredentialAuth(creds, nil, "/app"), "Bearer app-token", true, 200},
		{"refused control", srv.instanceAuth("control-token"), "Bearer wrong-token", false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(tc.auth)
			called := false
			router.GET("/app", func(c *gin.Context) {
				called = true
				request, release := credentialScopedRequest(c)
				defer release()
				if (request.Header.Get("Authorization") != "") != tc.wantHeader {
					t.Error("outbound application authorization presence differs")
				}
				request.Header.Set("X-App-Header", "outbound")
				if c.Request.Header.Get("X-App-Header") != "inbound" && !tc.wantHeader {
					t.Error("outbound projection shares authenticated request headers")
				}
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/app", nil)
			req.Header.Set("Authorization", tc.header)
			req.Header.Set("X-App-Header", "inbound")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.wantStatus || called != (tc.wantStatus == 200) {
				t.Fatalf("status = %d, handler called = %t", response.Code, called)
			}
			if req.Header.Get("Authorization") != tc.header {
				t.Error("original request authorization changed")
			}
		})
	}
}
