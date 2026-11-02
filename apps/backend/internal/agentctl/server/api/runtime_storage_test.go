package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestDeliveryStorageRuntimeStorageRouteReturnsAdvisory(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/storage", nil)
	server.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var snapshot streams.RuntimeStorageSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != streams.RuntimeStorageSnapshotVersion {
		t.Fatalf("version = %d", snapshot.Version)
	}
	if snapshot.StorageKind != streams.RuntimeStorageKindWorkspace || snapshot.Filesystem == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestDeliveryStorageStatusIncludesRuntimeStorage(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
	manager := process.NewManager(cfg, log)
	first := manager.RuntimeStorage(t.Context())
	server := NewServer(cfg, manager, nil, nil, log)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	server.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		RuntimeStorage *streams.RuntimeStorageSnapshot `json:"runtime_storage"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RuntimeStorage == nil || response.RuntimeStorage.Filesystem == nil {
		t.Fatalf("runtime storage = %+v", response.RuntimeStorage)
	}
	if !response.RuntimeStorage.Filesystem.ObservedAt.Equal(first.Filesystem.ObservedAt) {
		t.Fatal("ordinary status performed a fresh advisory read instead of using the cache")
	}
}

func TestDeliveryStorageStatusOmitsUnsampledRuntimeStorage(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	server.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		RuntimeStorage *streams.RuntimeStorageSnapshot `json:"runtime_storage"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RuntimeStorage != nil {
		t.Fatalf("unsampled status fabricated advisory capacity: %+v", response.RuntimeStorage)
	}
}
