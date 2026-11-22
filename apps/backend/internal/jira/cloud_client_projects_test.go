package jira

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// Provider fixtures follow the Server 9.12 project array and Cloud v3 values
// contracts. Unexpected routes return 404 rather than accepting either shape.
const serverProjectsJSON = `[{"id":"710","key":"OPS","name":"Operations","self":"https://jira.example/project/OPS"},{"id":"204","key":"APP","name":"Applications"}]`

var expectedContractProjects = []JiraProject{
	{ID: "710", Key: "OPS", Name: "Operations"},
	{ID: "204", Key: "APP", Name: "Applications"},
}

func projectContractServer(t *testing.T, uri, auth string, status int, body string) *httptest.Server {
	t.Helper()
	return newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.RequestURI() != uri {
			t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.RequestURI(), uri)
			http.Error(w, `{"errorMessages":["Unknown project resource"]}`, http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != auth {
			t.Errorf("authorization = %q, want %q", got, auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// @covers AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.1
// @covers AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.3
func TestCloudClient_ListProjects_ServerContract(t *testing.T) {
	ts := projectContractServer(t, "/rest/api/2/project", "Bearer fixture-pat", http.StatusOK, serverProjectsJSON)
	client := NewCloudClient(&JiraConfig{
		SiteURL: ts.URL, InstanceType: InstanceTypeServer, AuthMethod: AuthMethodPAT,
	}, "fixture-pat")
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("Server project listing: %v", err)
	}
	if !reflect.DeepEqual(projects, expectedContractProjects) {
		t.Fatalf("projects = %#v, want %#v", projects, expectedContractProjects)
	}
}

func TestCloudClient_ListProjects_ServerTransportControl(t *testing.T) {
	ts := projectContractServer(t, "/rest/api/2/project", "Bearer fixture-pat", http.StatusOK, serverProjectsJSON)
	client := NewCloudClient(&JiraConfig{
		SiteURL: ts.URL, InstanceType: InstanceTypeServer, AuthMethod: AuthMethodPAT,
	}, "fixture-pat")
	var projects []JiraProject
	if err := client.do(context.Background(), http.MethodGet, "/rest/api/2/project", nil, &projects); err != nil {
		t.Fatalf("documented Server transport: %v", err)
	}
	if !reflect.DeepEqual(projects, expectedContractProjects) {
		t.Fatalf("projects = %#v, want %#v", projects, expectedContractProjects)
	}
}

// @covers AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.2
func TestCloudClient_ListProjects_CloudContract(t *testing.T) {
	for _, instance := range []string{InstanceTypeCloud, ""} {
		t.Run("instance="+instance, func(t *testing.T) {
			ts := projectContractServer(t, "/rest/api/3/project/search?maxResults=200",
				"Basic dXNlckBleGFtcGxlLmNvbTpmaXh0dXJlLXRva2Vu", http.StatusOK,
				`{"values":`+serverProjectsJSON+`,"isLast":true,"total":2}`)
			client := NewCloudClient(&JiraConfig{
				SiteURL: ts.URL, InstanceType: instance, AuthMethod: AuthMethodAPIToken, Email: "user@example.com",
			}, "fixture-token")
			projects, err := client.ListProjects(context.Background())
			if err != nil {
				t.Fatalf("Cloud project listing: %v", err)
			}
			if !reflect.DeepEqual(projects, expectedContractProjects) {
				t.Fatalf("projects = %#v, want %#v", projects, expectedContractProjects)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.3
func TestCloudClient_ListProjects_ContextPath(t *testing.T) {
	for _, tc := range []struct {
		instance string
		uri      string
		body     string
		method   string
		auth     string
	}{
		{InstanceTypeServer, "/jira/rest/api/2/project", serverProjectsJSON, AuthMethodPAT, "Bearer fixture-pat"},
		{InstanceTypeCloud, "/jira/rest/api/3/project/search?maxResults=200", `{"values":` + serverProjectsJSON + `}`, AuthMethodAPIToken, "Basic dXNlckBleGFtcGxlLmNvbTpmaXh0dXJlLXBhdA=="},
	} {
		t.Run(tc.instance, func(t *testing.T) {
			ts := projectContractServer(t, tc.uri, tc.auth, http.StatusOK, tc.body)
			client := NewCloudClient(&JiraConfig{
				SiteURL: ts.URL + "/jira/", InstanceType: tc.instance, AuthMethod: tc.method, Email: "user@example.com",
			}, "fixture-pat")
			projects, err := client.ListProjects(context.Background())
			if err != nil || !reflect.DeepEqual(projects, expectedContractProjects) {
				t.Fatalf("context-path projects = %#v, error = %v", projects, err)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.4
func TestCloudClient_ListProjects_EmptyAndErrors(t *testing.T) {
	for _, provider := range []struct {
		instance string
		uri      string
		empty    string
		method   string
		auth     string
	}{
		{InstanceTypeServer, "/rest/api/2/project", `[]`, AuthMethodPAT, "Bearer fixture-pat"},
		{InstanceTypeCloud, "/rest/api/3/project/search?maxResults=200", `{"values":[]}`, AuthMethodAPIToken, "Basic dXNlckBleGFtcGxlLmNvbTpmaXh0dXJlLXBhdA=="},
	} {
		t.Run(provider.instance, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				status int
				body   string
			}{
				{"empty", http.StatusOK, provider.empty},
				{"forbidden", http.StatusForbidden, `{"errorMessages":["Project access denied"]}`},
				{"unavailable", http.StatusServiceUnavailable, `{"errorMessages":["Project service unavailable"]}`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ts := projectContractServer(t, provider.uri, provider.auth, tc.status, tc.body)
					client := NewCloudClient(&JiraConfig{
						SiteURL: ts.URL, InstanceType: provider.instance, AuthMethod: provider.method, Email: "user@example.com",
					}, "fixture-pat")
					projects, err := client.ListProjects(context.Background())
					if tc.status == http.StatusOK {
						if err != nil || projects == nil || len(projects) != 0 {
							t.Fatalf("empty projects = %#v, error = %v", projects, err)
						}
						return
					}
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status || apiErr.Message != tc.body {
						t.Fatalf("error = %#v, want status %d and body %s", err, tc.status, tc.body)
					}
					if projects != nil {
						t.Fatalf("failed request returned projects: %#v", projects)
					}
				})
			}
		})
	}
}
