package database

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/kandev/kandev/internal/system/maintenance"
	"testing"
)

func TestMaintenanceCanceledBehindRetentionLease(t *testing.T) {
	for _, name := range []string{"vacuum", "optimize", "reset"} {
		t.Run(name, func(t *testing.T) {
			s, _, _, _ := newTestService(t)
			release, ok := maintenance.ForPool(s.pool).TryAcquire()
			if !ok {
				t.Fatal("lease unavailable")
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			called := false
			s.OrchestratorShutdown = func() { called = true }
			var err error
			switch name {
			case "vacuum":
				_, err = s.runVacuum(ctx)
			case "optimize":
				_, err = s.runOptimize(ctx)
			case "reset":
				_, err = s.runFactoryReset(ctx)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("want cancellation, got %v", err)
			}
			if called {
				t.Fatal("reset started without admission")
			}
		})
	}
}

func TestResetHoldsMaintenanceAdmissionThroughQuiescence(t *testing.T) {
	s, _, _, _ := newTestService(t)
	stopped := errors.New("stop before reset")
	s.DatabaseQuiesce = func() error {
		if release, ok := maintenance.ForPool(s.pool).TryAcquire(); ok {
			release()
			t.Error("reset did not own admission")
		}
		return stopped
	}
	_, err := s.runFactoryReset(context.Background())
	if !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	release, ok := maintenance.ForPool(s.pool).TryAcquire()
	if !ok {
		t.Fatal("failed reset leaked admission")
	}
	release()
}

func TestHTTPMaintenanceSurvivesAcceptedRequestCancellation(t *testing.T) {
	for _, name := range []string{"vacuum", "optimize", "reset"} {
		t.Run(name, func(t *testing.T) {
			s, tracker, _, _ := newTestService(t)
			release, ok := maintenance.ForPool(s.pool).TryAcquire()
			if !ok {
				t.Fatal("lease unavailable")
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			router := gin.New()
			switch name {
			case "vacuum":
				router.POST("/", HandleVacuum(s))
			case "optimize":
				router.POST("/", HandleOptimize(s))
			case "reset":
				router.POST("/", HandleReset(s))
			}
			request := httpPost(t, "/", `{"confirm":"RESET"}`).WithContext(ctx)
			response := serveHTTP(router, request)
			if response.Code != 202 {
				t.Fatalf("response: %d %s", response.Code, response.Body.String())
			}
			var body struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			cancel()
			release()
			waitForState(t, tracker, body.JobID, jobs.StateSucceeded)
		})
	}
}

func TestAcceptedMaintenanceAPIsSurviveCallerCancellation(t *testing.T) {
	for _, name := range []string{"vacuum", "optimize", "reset"} {
		t.Run(name, func(t *testing.T) {
			s, tracker, _, _ := newTestService(t)
			release, ok := maintenance.ForPool(s.pool).TryAcquire()
			if !ok {
				t.Fatal("lease unavailable")
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var id string
			switch name {
			case "vacuum":
				id = s.Vacuum(ctx)
			case "optimize":
				id = s.Optimize(ctx)
			case "reset":
				var err error
				id, err = s.FactoryReset(ctx, resetConfirmToken)
				if err != nil {
					t.Fatal(err)
				}
			}
			cancel()
			release()
			waitForState(t, tracker, id, jobs.StateSucceeded)
		})
	}
}
