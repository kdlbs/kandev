package sshconnections

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func managerRouter(s *Service, identity *authn.Identity) *gin.Engine {
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) { authn.SetOnGin(c, *identity); c.Next() })
	}
	RegisterRoutes(router, s)
	return router
}
func TestManagerHTTPGateAndPrivateKeyBoundary(t *testing.T) {
	for _, tt := range []struct {
		enabled  bool
		identity *authn.Identity
		want     int
	}{
		{false, nil, 404}, {true, nil, 401}, {true, &authn.Identity{UserID: "member", Role: authn.RoleMember}, 403},
	} {
		router := managerRouter(NewService(tt.enabled, nil, nil), tt.identity)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ssh/managed/identities", strings.NewReader(`{}`)))
		if rec.Code != tt.want {
			t.Fatalf("status = %d, want %d", rec.Code, tt.want)
		}
	}
	s, _ := identityService(t)
	router := managerRouter(s, &authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
	req := CreateIdentityRequest{RequestID: uuid.NewString(), Label: "generated", Kind: "generate"}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ssh/managed/identities", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, request)
	if rec.Code != 200 {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") || strings.Contains(rec.Body.String(), "private_key") {
		t.Fatal("returned private key material")
	}
	var identity Identity
	if err = json.Unmarshal(rec.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.PublicKey == "" {
		t.Fatal("missing public key")
	}
}

func TestConnectionHTTPReadAndDelete(t *testing.T) {
	s, ctx := identityService(t)
	created, err := s.SaveConnection(ctx, SaveConnectionRequest{RequestID: uuid.NewString(), Connection: Connection{Name: "draft", Port: 22}})
	if err != nil {
		t.Fatal(err)
	}
	router := managerRouter(s, &authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
	for _, path := range []string{"/api/v1/ssh/managed/connections", "/api/v1/ssh/managed/connections/" + created.Destination.ID} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 {
			t.Fatalf("read status %d", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/ssh/managed/connections/"+created.Destination.ID, strings.NewReader(`{"expected_version":1}`)))
	if rec.Code != 200 {
		t.Fatalf("delete status %d", rec.Code)
	}
}

func TestManagedSSHResponsesUseDestinationWireKey(t *testing.T) {
	value := Route{Destination: Connection{Host: "fixture.example"}}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response["destination"]; !ok {
		t.Fatalf("missing destination wire key: %s", body)
	}
	if _, ok := response["Destination"]; ok {
		t.Fatal("exported Go field name leaked into wire response")
	}
}

func TestCutoverRetiresAdoptionEndpoints(t *testing.T) {
	router := managerRouter(NewService(true, nil, nil), nil)
	for path, method := range map[string]string{"adoption-preview": "GET", "adoption-drafts": "POST", "adopt-connection": "POST"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/ssh/managed/executors/old/"+path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s returned %d", path, rec.Code)
		}
	}
}
