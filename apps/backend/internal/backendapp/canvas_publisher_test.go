package backendapp

import (
	"encoding/json"
	"testing"
)

func TestCanvasInstallRequestDoesNotAcceptBrowserPublisherEvidence(t *testing.T) {
	var request canvasInstallRequest
	if err := json.Unmarshal([]byte(`{
		"workspace_id":"workspace-1",
		"origin_kind":"upload",
		"source_id":"official",
		"repository_url":"https://github.com/kdlbs/kandev",
		"publisher":{"schema_version":1,"repository_id":"1","owner_id":"2","login":"kdlbs","repository":"kdlbs/kandev","official":true},
		"publisher_identity":{"status":"verified","login":"kdlbs","official":true}
	}`), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	serviceRequest := request.serviceRequest("user-1", []byte("bundle"))
	if serviceRequest.PublisherProvenance != nil {
		t.Fatalf("browser supplied publisher provenance = %+v, want nil", serviceRequest.PublisherProvenance)
	}
	if serviceRequest.SourceID != "official" || serviceRequest.RepositoryURL == "" {
		t.Fatalf("source metadata was not preserved as informational input: %+v", serviceRequest)
	}
}
