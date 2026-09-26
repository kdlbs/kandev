package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublisherTrustBoundary(t *testing.T) {
	evidence := `"publisher":{"schema_version":1,"repository_id":"123","owner_id":"456","login":"acme","repository":"acme/example","official":false}`
	index := `{"schema_version":1,"source":{"name":"forged","url":"https://forged.example/index.json"},"plugins":[{"id":"example","name":"Example","version":"1.0.0","package_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","package_url":"https://cdn.example/example.tar.gz",` + evidence + `}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(server.Close)

	s := newTestService(t)
	if err := s.store.EnsureBuiltin(OfficialSourceName, server.URL); err != nil {
		t.Fatal(err)
	}
	result, err := s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if got := result.Plugins[0].PublisherIdentity; got == nil || got.Status != "unverified" {
		t.Fatalf("URL-overridden builtin projected forged verification: %+v", got)
	}

	// The test-only canonical reference simulates the production transport
	// binding while keeping the HTTP fixture local.
	s.canonicalOfficialURL = server.URL
	s.Refresh()
	result, err = s.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("canonical catalog: %v", err)
	}
	if got := result.Plugins[0].PublisherIdentity; got == nil || got.Status != "verified" || got.Login != "acme" {
		t.Fatalf("canonical evidence was not projected: %+v", got)
	}

	s2 := newTestService(t)
	custom, err := s2.store.Add("Custom", server.URL+"/custom")
	if err != nil {
		t.Fatal(err)
	}
	customResult, err := s2.Catalog(context.Background(), nil)
	if err != nil {
		t.Fatalf("custom catalog: %v", err)
	}
	if got := customResult.Plugins[0].PublisherIdentity; got == nil || got.Status != "unverified" {
		t.Fatalf("custom source projected verification: %+v (source=%+v)", got, custom)
	}
}
