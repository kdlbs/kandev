package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPostHogSinkPayloadShape(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink := newPostHogSink(server.URL, "phc_test_key")
	events := []Event{
		{Name: EventTaskCreated, Timestamp: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
			Properties: map[string]string{"app_version": "1.0.0"}},
	}
	if err := sink.Send(context.Background(), "install-uuid-1", events); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/batch/" {
		t.Fatalf("expected /batch/ path, got %q", gotPath)
	}
	if gotBody["api_key"] != "phc_test_key" {
		t.Fatalf("api_key missing: %v", gotBody)
	}
	batch, ok := gotBody["batch"].([]any)
	if !ok || len(batch) != 1 {
		t.Fatalf("expected batch of 1, got %v", gotBody["batch"])
	}
	item := batch[0].(map[string]any)
	if item["event"] != EventTaskCreated || item["distinct_id"] != "install-uuid-1" {
		t.Fatalf("unexpected item: %v", item)
	}
	if item["timestamp"] != "2026-07-15T12:00:00Z" {
		t.Fatalf("unexpected timestamp: %v", item["timestamp"])
	}
	properties := item["properties"].(map[string]any)
	if properties["$process_person_profile"] != false {
		t.Fatalf("events must be anonymous-mode ($process_person_profile: false): %v", properties)
	}
	if properties["app_version"] != "1.0.0" || properties["$lib"] != "kandev" {
		t.Fatalf("properties not merged: %v", properties)
	}
}

func TestPostHogSinkErrorsOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	sink := newPostHogSink(server.URL, "phc_test_key")
	err := sink.Send(context.Background(), "id", []Event{{Name: EventTaskCreated, Timestamp: time.Now()}})
	if err == nil {
		t.Fatal("expected error on 502")
	}
}
