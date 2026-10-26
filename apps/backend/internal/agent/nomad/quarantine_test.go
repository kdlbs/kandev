package nomad

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quarantineClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNomadWorkerQuarantineSetNodeIneligibleSendsOnlyIneligible(t *testing.T) {
	var body string
	client := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/node/node-b/eligibility" || r.Header.Get("X-Nomad-Token") != "fixture-token" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = io.WriteString(w, `{}`)
	})
	if err := client.SetNodeIneligible(t.Context(), "node-b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"Eligibility":"ineligible"`) || strings.Contains(body, `"eligible"`) || !strings.Contains(body, `"NodeID":"node-b"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestNomadWorkerQuarantineErrorsDoNotEchoBodyOrToken(t *testing.T) {
	client := quarantineClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "secret-body fixture-token")
	})
	checks := map[string]error{
		"set": client.SetNodeIneligible(t.Context(), "node-b"),
	}
	_, checks["get"] = client.GetNode(t.Context(), "node-b")
	_, _, checks["read"] = client.ReadQuarantineMarker(t.Context(), "default", "node-b")
	_, _, checks["put"] = client.PutQuarantineMarkerCAS(t.Context(), "default", testMarker())
	for name, err := range checks {
		if err == nil || strings.Contains(err.Error(), "secret-body") || strings.Contains(err.Error(), "fixture-token") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}

func TestNomadWorkerQuarantineTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := client.SetNodeIneligible(t.Context(), "node-b"); err == nil {
		t.Fatal("expected transport error")
	}
	if _, _, err := client.PutQuarantineMarkerCAS(t.Context(), "default", testMarker()); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestNomadWorkerQuarantineGetNodeAndDedicatedCheck(t *testing.T) {
	client := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/node/node-b" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"ID":"node-b","Datacenter":"dc1","Status":"ready","SchedulingEligibility":"eligible","Attributes":{"kernel.name":"darwin","cpu.arch":"arm64"}}`)
	})
	node, err := client.GetNode(t.Context(), "node-b")
	if err != nil || node.Status != "ready" || node.SchedulingEligibility != "eligible" {
		t.Fatalf("GetNode = %+v, %v", node, err)
	}
	profile := validTestProfileConfig()
	profile.NodeID, profile.Datacenter, profile.Platform = "node-b", "dc1", "darwin/arm64"
	if err := CheckDedicatedNode(profile, node); err != nil {
		t.Fatalf("matching node rejected: %v", err)
	}
	aarch := *node
	aarch.Attributes = map[string]string{"kernel.name": "darwin", "cpu.arch": "aarch64"}
	if err := CheckDedicatedNode(profile, &aarch); err != nil {
		t.Fatalf("aarch64 rejected: %v", err)
	}
	for name, mutate := range map[string]func(*NodeRecord){
		"node":       func(n *NodeRecord) { n.ID = "other" },
		"datacenter": func(n *NodeRecord) { n.Datacenter = "dc2" },
		"kernel":     func(n *NodeRecord) { n.Attributes = map[string]string{"kernel.name": "linux", "cpu.arch": "arm64"} },
		"arch":       func(n *NodeRecord) { n.Attributes = map[string]string{"kernel.name": "darwin", "cpu.arch": "amd64"} },
		"missing":    func(n *NodeRecord) { n.Attributes = nil },
	} {
		bad := *node
		mutate(&bad)
		if err := CheckDedicatedNode(profile, &bad); err == nil {
			t.Fatalf("%s mismatch accepted", name)
		}
	}
	if err := CheckDedicatedNode(profile, nil); err == nil {
		t.Fatal("nil node accepted")
	}
}

func testMarker() QuarantineMarker {
	return QuarantineMarker{Version: 1, NodeID: "node-b", EnvironmentID: "env-1", OperationID: "op-1", JobID: "job-1", Reason: "cleanup_unresolved", Generation: 3}
}

func TestNomadWorkerQuarantineMarkerCreate(t *testing.T) {
	var query, body string
	client := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/var/kandev/quarantine/node-b" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = io.WriteString(w, `{}`)
	})
	existing, created, err := client.PutQuarantineMarkerCAS(t.Context(), "default", testMarker())
	if err != nil || !created || existing != nil {
		t.Fatalf("put = %+v, %v, %v", existing, created, err)
	}
	if !strings.Contains(query, "cas=0") || !strings.Contains(query, "namespace=default") || !strings.Contains(body, `"marker"`) || !strings.Contains(body, "kandev/quarantine/node-b") {
		t.Fatalf("query=%s body=%s", query, body)
	}
	if _, _, err := client.PutQuarantineMarkerCAS(t.Context(), "default", QuarantineMarker{NodeID: "../x"}); err == nil {
		t.Fatal("unsafe node ID accepted")
	}
}

func TestNomadWorkerQuarantineMarkerConflictReturnsExistingWithoutOverwrite(t *testing.T) {
	puts := 0
	client := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"ModifyIndex":9,"Items":{"marker":"{\"version\":1,\"node_id\":\"node-b\",\"environment_id\":\"env-0\",\"generation\":1}"}}`)
			return
		}
		puts++
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"ModifyIndex":9,"Items":{"marker":"{\"version\":1,\"node_id\":\"node-b\",\"environment_id\":\"env-0\",\"generation\":1}"}}`)
	})
	existing, created, err := client.PutQuarantineMarkerCAS(t.Context(), "default", testMarker())
	if err != nil || created || existing == nil || existing.EnvironmentID != "env-0" || puts != 1 {
		t.Fatalf("conflict = %+v, %v, %v, puts=%d", existing, created, err, puts)
	}
}

func TestNomadWorkerQuarantineMarkerConflictFallsBackToRead(t *testing.T) {
	client := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"ModifyIndex":4,"Items":{"marker":"{\"version\":1,\"environment_id\":\"env-9\"}"}}`)
			return
		}
		w.WriteHeader(http.StatusConflict)
	})
	existing, created, err := client.PutQuarantineMarkerCAS(t.Context(), "default", testMarker())
	if err != nil || created || existing == nil || existing.EnvironmentID != "env-9" {
		t.Fatalf("conflict = %+v, %v, %v", existing, created, err)
	}
}

func TestNomadWorkerQuarantineReadMarker(t *testing.T) {
	notFound := quarantineClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	marker, index, err := notFound.ReadQuarantineMarker(t.Context(), "default", "node-b")
	if err != nil || marker != nil || index != 0 {
		t.Fatalf("404 read = %+v, %d, %v", marker, index, err)
	}
	found := quarantineClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespace") != "default" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"ModifyIndex":12,"Items":{"marker":"{\"version\":1,\"node_id\":\"node-b\",\"generation\":3}"}}`)
	})
	marker, index, err = found.ReadQuarantineMarker(t.Context(), "default", "node-b")
	if err != nil || marker == nil || marker.Generation != 3 || index != 12 {
		t.Fatalf("read = %+v, %d, %v", marker, index, err)
	}
	if _, _, err := found.ReadQuarantineMarker(t.Context(), "default", "bad/id"); err == nil {
		t.Fatal("unsafe node ID accepted")
	}
}
