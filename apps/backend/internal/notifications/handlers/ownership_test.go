package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/notifications/controller"
	"github.com/kandev/kandev/internal/notifications/dto"
	"github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// testUserHeader names the user a request authenticates as. The middleware
// below stands in for auth/httpmw so these tests exercise the real handler,
// controller, service and SQLite store together.
const testUserHeader = "X-Test-User"

func newProviderAPI(t *testing.T) *gin.Engine {
	t.Helper()
	// Keep the auto-provisioned system provider out: SystemProvider.Available()
	// is true on desktop platforms and would vary the seeded provider count.
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "true")
	gin.SetMode(gin.TestMode)

	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, cleanup, err := notificationstore.Provide(context.Background(), database, database)
	if err != nil {
		t.Fatalf("create notification store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID := c.GetHeader(testUserHeader); userID != "" {
			authn.SetOnGin(c, authn.Identity{UserID: userID, Role: authn.RoleMember})
		}
		c.Next()
	})
	RegisterRoutes(router, controller.NewController(service.NewService(repo, nil, nil, log, nil)), log)
	return router
}

func callAs(t *testing.T, router *gin.Engine, userID, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request.Header.Set(testUserHeader, userID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func createProviderAs(t *testing.T, router *gin.Engine, userID, name, webhook string) dto.NotificationProviderDTO {
	t.Helper()
	body, err := json.Marshal(dto.UpsertProviderRequest{
		Name:   name,
		Type:   "apprise",
		Config: map[string]interface{}{"urls": webhook},
	})
	if err != nil {
		t.Fatalf("encode create request: %v", err)
	}
	response := callAs(t, router, userID, http.MethodPost, "/api/v1/notification-providers", string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("create provider as %s = %d: %s", userID, response.Code, response.Body.String())
	}
	var created dto.NotificationProviderDTO
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created provider: %v", err)
	}
	return created
}

func listProviderNamesAs(t *testing.T, router *gin.Engine, userID string) []string {
	t.Helper()
	response := callAs(t, router, userID, http.MethodGet, "/api/v1/notification-providers", "")
	if response.Code != http.StatusOK {
		t.Fatalf("list providers as %s = %d: %s", userID, response.Code, response.Body.String())
	}
	var listed dto.NotificationProvidersResponse
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	names := make([]string, 0, len(listed.Providers))
	for _, provider := range listed.Providers {
		names = append(names, provider.Name)
	}
	return names
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestProviderListShowsEachUserOnlyTheirOwnProviders(t *testing.T) {
	router := newProviderAPI(t)
	createProviderAs(t, router, "user-a", "a-webhook", "slack://user-a-token")
	createProviderAs(t, router, "user-b", "b-webhook", "slack://user-b-token")

	forA := listProviderNamesAs(t, router, "user-a")
	forB := listProviderNamesAs(t, router, "user-b")

	if !contains(forA, "a-webhook") || contains(forA, "b-webhook") {
		t.Fatalf("user-a sees %#v, want only their own webhook", forA)
	}
	if !contains(forB, "b-webhook") || contains(forB, "a-webhook") {
		t.Fatalf("user-b sees %#v, want only their own webhook", forB)
	}
}

func TestForeignProviderIsIndistinguishableFromAMissingOne(t *testing.T) {
	router := newProviderAPI(t)
	owned := createProviderAs(t, router, "user-a", "a-webhook", "slack://user-a-token")

	for _, tc := range []struct {
		name, method, suffix, body string
	}{
		{"update", http.MethodPatch, "", `{"name":"stolen"}`},
		{"delete", http.MethodDelete, "", ""},
		{"test", http.MethodPost, "/test", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			foreign := callAs(t, router, "user-b", tc.method, "/api/v1/notification-providers/"+owned.ID+tc.suffix, tc.body)
			missing := callAs(t, router, "user-b", tc.method, "/api/v1/notification-providers/no-such-provider"+tc.suffix, tc.body)

			if foreign.Code != http.StatusNotFound {
				t.Fatalf("%s on a foreign provider = %d: %s", tc.name, foreign.Code, foreign.Body.String())
			}
			if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
				t.Fatalf("%s foreign response (%d %s) differs from missing (%d %s)",
					tc.name, foreign.Code, foreign.Body.String(), missing.Code, missing.Body.String())
			}
		})
	}

	// No mutation may have escaped: the owner still sees the original row.
	if names := listProviderNamesAs(t, router, "user-a"); !contains(names, "a-webhook") || contains(names, "stolen") {
		t.Fatalf("user-a providers = %#v, want the untouched a-webhook", names)
	}
}

func TestOwnerCanStillUpdateAndDeleteTheirOwnProvider(t *testing.T) {
	router := newProviderAPI(t)
	owned := createProviderAs(t, router, "user-a", "a-webhook", "slack://user-a-token")

	updated := callAs(t, router, "user-a", http.MethodPatch, "/api/v1/notification-providers/"+owned.ID, `{"name":"renamed"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("owner update = %d: %s", updated.Code, updated.Body.String())
	}
	if names := listProviderNamesAs(t, router, "user-a"); !contains(names, "renamed") {
		t.Fatalf("user-a providers = %#v, want the renamed provider", names)
	}

	deleted := callAs(t, router, "user-a", http.MethodDelete, "/api/v1/notification-providers/"+owned.ID, "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("owner delete = %d: %s", deleted.Code, deleted.Body.String())
	}
	if names := listProviderNamesAs(t, router, "user-a"); contains(names, "renamed") {
		t.Fatalf("user-a providers = %#v, want the provider gone", names)
	}
}

func TestRequestsWithoutAnIdentityUseTheDefaultUser(t *testing.T) {
	router := newProviderAPI(t)
	createProviderAs(t, router, "", "unauthenticated-webhook", "slack://single-user")

	// With authentication disabled the row lands on the same default user
	// single-user installs have always used.
	if names := listProviderNamesAs(t, router, userstore.DefaultUserID); !contains(names, "unauthenticated-webhook") {
		t.Fatalf("default user providers = %#v, want the row created without an identity", names)
	}
	if names := listProviderNamesAs(t, router, ""); !contains(names, "unauthenticated-webhook") {
		t.Fatalf("unauthenticated list = %#v, want the same rows", names)
	}
}

func TestSyntheticIdentityIsTreatedAsTheDefaultUser(t *testing.T) {
	router := newProviderAPI(t)
	createProviderAs(t, router, userstore.DefaultUserID, "single-user-webhook", "slack://single-user")

	// auth/httpmw injects a synthetic admin while authentication is disabled;
	// it must resolve to the default row, not to its own user ID.
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "synthetic-admin", Role: authn.RoleAdmin, Synthetic: true,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/notification-providers", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var listed dto.NotificationProvidersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	names := make([]string, 0, len(listed.Providers))
	for _, provider := range listed.Providers {
		names = append(names, provider.Name)
	}
	if !contains(names, "single-user-webhook") {
		t.Fatalf("synthetic identity sees %#v, want the default user's rows", names)
	}
}
