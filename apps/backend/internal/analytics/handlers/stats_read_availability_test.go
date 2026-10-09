package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/analytics/models"
	analyticsservice "github.com/kandev/kandev/internal/analytics/service"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
)

const availabilityWorkspaceID = statsReferenceWorkspaceID

func TestAnalyticsReadAvailability(t *testing.T) {
	fixture := newStatsReferenceFixture(t, statsAvailabilityWorkload, false)
	health := newAnalyticsAvailabilityHealth(t, fixture)
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial health check failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := authzTestLogger(t)
	RegisterStatsRoutes(router, fixture.repo, statsReferenceAuthorizer{}, log)

	operationCtx, cancelOperations := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelOperations()
	httpDone := make(chan int, 2)
	for _, section := range []string{"global", "repositories"} {
		section := section
		go func() {
			request := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/workspaces/"+availabilityWorkspaceID+"/stats/"+section+"?range=all",
				nil,
			).WithContext(operationCtx)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			httpDone <- response.Code
		}()
	}

	waitForReaderConnections(t, fixture, 2)
	if inUse := fixture.reader.Stats().InUse; inUse < 2 {
		t.Fatalf("reader occupancy = %d, want both admitted HTTP operations active", inUse)
	}

	serviceCtx, cancelService := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelService()
	serviceDone := make(chan error, 1)
	go func() {
		service := analyticsservice.New(fixture.repo)
		_, serviceErr := service.ListSessionCodeStats(serviceCtx, models.SessionCodeStatsFilter{
			WorkspaceIDs: []string{availabilityWorkspaceID},
			Limit:        1,
		})
		serviceDone <- serviceErr
	}()

	var taskCount int
	readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
	defer cancelRead()
	if err := fixture.reader.GetContext(readCtx, &taskCount, `SELECT COUNT(*) FROM tasks`); err != nil {
		t.Fatalf("normal read blocked by analytics occupancy: %v", err)
	}
	if taskCount == 0 {
		t.Fatal("reference workload was not visible to the reader pool")
	}
	if err := health.Check(readCtx); err != nil {
		t.Fatalf("required-store health probe blocked by analytics occupancy: %v", err)
	}
	if !health.Healthy() {
		t.Fatal("required-store health became unhealthy during analytics occupancy")
	}

	select {
	case serviceErr := <-serviceDone:
		if !errors.Is(serviceErr, context.DeadlineExceeded) {
			t.Fatalf("queued service/plugin analytics error = %v, want context deadline", serviceErr)
		}
	case <-time.After(time.Second):
		t.Fatal("queued service/plugin analytics operation did not observe its deadline")
	}

	for range 2 {
		select {
		case status := <-httpDone:
			if status != http.StatusOK {
				t.Fatalf("HTTP analytics operation status = %d, want 200", status)
			}
		case <-operationCtx.Done():
			t.Fatalf("HTTP analytics operation did not finish: %v", operationCtx.Err())
		}
	}
}

func TestAnalyticsReadAvailabilityPreservesMissingStoreFailure(t *testing.T) {
	fixture := newStatsReferenceFixture(t, statsAvailabilityWorkload, false)
	health := newAnalyticsAvailabilityHealth(t, fixture)
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial health check failed: %v", err)
	}
	if _, err := fixture.writer.Exec(`DROP TABLE tasks`); err != nil {
		t.Fatalf("drop tasks: %v", err)
	}

	if err := health.Check(context.Background()); err == nil {
		t.Fatal("health check after dropping a required table returned nil")
	}
	if health.Healthy() {
		t.Fatal("health remained healthy after a required table was dropped")
	}
}

func newAnalyticsAvailabilityHealth(t *testing.T, fixture statsReferenceFixture) *requiredstores.Health {
	t.Helper()
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID:             "analytics",
		OwnerPackage:   "internal/analytics/repository",
		RequiredTables: []string{"tasks"},
		Sweep:          startup.StepStoresRepositories,
	}})
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}
	if err := tracker.RecordSuccess("analytics"); err != nil {
		t.Fatalf("RecordSuccess failed: %v", err)
	}
	return requiredstores.NewHealth(tracker, db.NewPool(fixture.writer, fixture.reader), nil)
}

func waitForReaderConnections(t *testing.T, fixture statsReferenceFixture, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fixture.reader.Stats().InUse < want {
		if time.Now().After(deadline) {
			t.Fatalf("reader occupancy = %d, want at least %d", fixture.reader.Stats().InUse, want)
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
}
