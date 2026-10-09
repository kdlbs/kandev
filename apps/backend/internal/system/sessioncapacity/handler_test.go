package sessioncapacity

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerGetAndPatchSessionCapacitySettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := &memoryRawStore{}
	service := NewService(NewStore(raw), &fakeTarget{}, Environment{}, testLogger(t))
	router := sessionCapacityRouter(service)

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/system/session-capacity/settings", nil,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", getResponse.Code, getResponse.Body.String())
	}
	var initial Response
	if err := json.Unmarshal(getResponse.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if initial.Settings != (Settings{Enabled: false, MaxSessions: DefaultMaxSessions}) {
		t.Fatalf("initial settings = %+v", initial.Settings)
	}

	patchRequest := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/session-capacity/settings",
		bytes.NewBufferString(`{"enabled":true,"max_sessions":6}`),
	)
	patchRequest.Header.Set("Content-Type", "application/json")
	patchResponse := httptest.NewRecorder()
	router.ServeHTTP(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body=%s", patchResponse.Code, patchResponse.Body.String())
	}
	var updated Response
	if err := json.Unmarshal(patchResponse.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode PATCH: %v", err)
	}
	if updated.Settings != (Settings{Enabled: true, MaxSessions: 6}) {
		t.Fatalf("updated settings = %+v", updated.Settings)
	}
}

func TestHandlerReturnsBadRequestForMalformedOrInvalidPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := sessionCapacityRouter(NewService(
		NewStore(&memoryRawStore{}), &fakeTarget{}, Environment{}, testLogger(t),
	))

	for _, body := range []string{
		"not-json",
		`null`,
		`{"enabled":null}`,
		`{"enabled":"yes"}`,
		`{"max_sessions":0}`,
		`{"max_sessions":1} trailing`,
	} {
		t.Run(body, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPatch, "/api/v1/system/session-capacity/settings",
				strings.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestHandlerReturnsConflictForEnvironmentLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := sessionCapacityRouter(NewService(
		NewStore(&memoryRawStore{}), &fakeTarget{},
		Environment{Value: "6", Present: true}, testLogger(t),
	))
	request := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/session-capacity/settings",
		strings.NewReader(`{"enabled":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerKeepsLoadSaveAndTargetErrorsGeneric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadFailure := errors.New("database secret should not be exposed")
	loadRouter := sessionCapacityRouter(NewService(
		NewStore(&memoryRawStore{getErr: loadFailure}), &fakeTarget{}, Environment{}, testLogger(t),
	))
	getResponse := httptest.NewRecorder()
	loadRouter.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/system/session-capacity/settings", nil,
	))
	if getResponse.Code != http.StatusInternalServerError || strings.Contains(getResponse.Body.String(), loadFailure.Error()) {
		t.Fatalf("load error response = %d %s", getResponse.Code, getResponse.Body.String())
	}

	saveFailure := errors.New("save secret should not be exposed")
	saveRouter := sessionCapacityRouter(NewService(
		NewStore(&memoryRawStore{saveErr: saveFailure}), &fakeTarget{}, Environment{}, testLogger(t),
	))
	saveRequest := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/session-capacity/settings",
		strings.NewReader(`{"enabled":true}`),
	)
	saveRequest.Header.Set("Content-Type", "application/json")
	saveResponse := httptest.NewRecorder()
	saveRouter.ServeHTTP(saveResponse, saveRequest)
	if saveResponse.Code != http.StatusInternalServerError || strings.Contains(saveResponse.Body.String(), saveFailure.Error()) {
		t.Fatalf("save error response = %d %s", saveResponse.Code, saveResponse.Body.String())
	}

	targetRouter := sessionCapacityRouter(NewService(
		NewStore(&memoryRawStore{}), nil, Environment{}, testLogger(t),
	))
	targetRequest := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/session-capacity/settings",
		strings.NewReader(`{"enabled":true}`),
	)
	targetRequest.Header.Set("Content-Type", "application/json")
	targetResponse := httptest.NewRecorder()
	targetRouter.ServeHTTP(targetResponse, targetRequest)
	if targetResponse.Code != http.StatusInternalServerError {
		t.Fatalf("target error status = %d, body=%s", targetResponse.Code, targetResponse.Body.String())
	}
}

func sessionCapacityRouter(service *Service) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, group, service)
	return router
}
