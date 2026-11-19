package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gatedImproveKandevCloner struct {
	path     string
	started  chan struct{}
	release  <-chan struct{}
	canceled chan struct{}
}

type pairedImproveKandevCloner struct {
	path    string
	started chan struct{}
	release <-chan struct{}
}

func (c pairedImproveKandevCloner) EnsureWorkspaceCloned(ctx context.Context, _, _, _, _, _ string) (string, error) {
	c.started <- struct{}{}
	select {
	case <-c.release:
		return c.path, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c gatedImproveKandevCloner) EnsureWorkspaceCloned(ctx context.Context, _, _, _, _, _ string) (string, error) {
	c.started <- struct{}{}
	select {
	case <-c.release:
		return c.path, nil
	case <-ctx.Done():
		if c.canceled != nil {
			c.canceled <- struct{}{}
		}
		return "", ctx.Err()
	}
}

type failOnceImproveKandevCloner struct {
	path  string
	calls atomic.Int32
}

func (c *failOnceImproveKandevCloner) EnsureWorkspaceCloned(context.Context, string, string, string, string, string) (string, error) {
	if c.calls.Add(1) == 1 {
		return "", errors.New("temporary clone failure")
	}
	return c.path, nil
}

type unexpectedImproveKandevCloner struct{ calls atomic.Int32 }

func (c *unexpectedImproveKandevCloner) EnsureWorkspaceCloned(context.Context, string, string, string, string, string) (string, error) {
	c.calls.Add(1)
	return "", errors.New("completed repository should have been reused")
}

type immediateImproveKandevGitHub struct{}

func (immediateImproveKandevGitHub) DescribeTaskGitCredentialPolicy(context.Context, string) (github.TaskGitCredentialPolicy, error) {
	return github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}, nil
}

func (immediateImproveKandevGitHub) ProbeContributionForkCapabilityForWorkspace(context.Context, string, string, string) (github.ContributionForkResolution, error) {
	return github.ContributionForkResolution{Status: github.ContributionForkStatusCreatable, ActorLogin: "automation"}, nil
}

type pairedImproveKandevGitHub struct {
	started chan struct{}
	release <-chan struct{}
}

func (g pairedImproveKandevGitHub) DescribeTaskGitCredentialPolicy(context.Context, string) (github.TaskGitCredentialPolicy, error) {
	return github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}, nil
}

func (g pairedImproveKandevGitHub) ProbeContributionForkCapabilityForWorkspace(ctx context.Context, _, _, _ string) (github.ContributionForkResolution, error) {
	g.started <- struct{}{}
	select {
	case <-g.release:
		return github.ContributionForkResolution{Status: github.ContributionForkStatusCreatable, ActorLogin: "automation"}, nil
	case <-ctx.Done():
		return github.ContributionForkResolution{}, ctx.Err()
	}
}

type gatedImproveKandevGitHub struct {
	started chan struct{}
	release <-chan struct{}
}

func (g gatedImproveKandevGitHub) DescribeTaskGitCredentialPolicy(context.Context, string) (github.TaskGitCredentialPolicy, error) {
	return github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}, nil
}

func (g gatedImproveKandevGitHub) ProbeContributionForkCapabilityForWorkspace(ctx context.Context, _, _, _ string) (github.ContributionForkResolution, error) {
	g.started <- struct{}{}
	select {
	case <-g.release:
		return github.ContributionForkResolution{Status: github.ContributionForkStatusCreatable, ActorLogin: "automation"}, nil
	case <-ctx.Done():
		return github.ContributionForkResolution{}, ctx.Err()
	}
}

func TestImproveKandevBootstrapAsyncDeadline(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	githubStarted := make(chan struct{}, 1)
	githubRelease := make(chan struct{})
	var releaseGitHub sync.Once
	defer releaseGitHub.Do(func() { close(githubRelease) })
	cloneStarted := make(chan struct{}, 1)
	cloneRelease := make(chan struct{})
	var releaseClone sync.Once
	defer releaseClone.Do(func() { close(cloneRelease) })
	handler := improvekandev.NewHandler(ts.TaskSvc,
		gatedImproveKandevCloner{path: repoPath, started: cloneStarted, release: cloneRelease},
		nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = handler.Stop() })
	handler.SetManagedGitHubForkProber(gatedImproveKandevGitHub{started: githubStarted, release: githubRelease})

	router := newImproveKandevTestRouter(handler)
	backend := httptest.NewServer(router)
	t.Cleanup(backend.Close)
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxyServer := httptest.NewServer(proxy)
	t.Cleanup(proxyServer.Close)
	client := proxyServer.Client()

	requestBody, err := json.Marshal(map[string]any{"create_workspace": true, "async": true})
	require.NoError(t, err)
	postDone := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		request, requestErr := http.NewRequest(http.MethodPost, proxyServer.URL+"/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(requestBody))
		if requestErr != nil {
			postDone <- struct {
				response *http.Response
				err      error
			}{err: requestErr}
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := client.Do(request)
		postDone <- struct {
			response *http.Response
			err      error
		}{response: response, err: requestErr}
	}()

	select {
	case <-githubStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap worker did not start the GitHub probe")
	}

	var admission *http.Response
	select {
	case result := <-postDone:
		require.NoError(t, result.err)
		admission = result.response
	case <-time.After(5 * time.Second):
		t.Fatal("async bootstrap did not acknowledge the request while GitHub was blocked")
	}
	defer closeImproveKandevResponseBody(t, admission)
	require.Equal(t, http.StatusAccepted, admission.StatusCode)
	require.Equal(t, "no-store", admission.Header.Get("Cache-Control"))
	var admitted struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.NewDecoder(admission.Body).Decode(&admitted))
	require.NotEmpty(t, admitted.JobID)

	statusURL := proxyServer.URL + "/api/v1/system/improve-kandev/bootstrap/" + admitted.JobID
	requestStatus := func() (*http.Response, error) {
		request, requestErr := http.NewRequest(http.MethodGet, statusURL, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		response, requestErr := client.Do(request)
		return response, requestErr
	}
	status, err := requestStatus()
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status.StatusCode)
	require.Equal(t, "no-store", status.Header.Get("Cache-Control"))
	closeImproveKandevResponseBody(t, status)

	releaseGitHub.Do(func() { close(githubRelease) })
	select {
	case <-cloneStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap worker did not continue to clone after GitHub completed")
	}
	status, err = requestStatus()
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status.StatusCode)
	closeImproveKandevResponseBody(t, status)

	releaseClone.Do(func() { close(cloneRelease) })
	var ready *http.Response
	var lastRequestErr error
	if !assert.Eventually(t, func() bool {
		response, requestErr := requestStatus()
		if requestErr != nil {
			lastRequestErr = requestErr
			return false
		}
		lastRequestErr = nil
		if response.StatusCode != http.StatusOK {
			closeImproveKandevResponseBody(t, response)
			return false
		}
		ready = response
		return true
	}, 10*time.Second, 25*time.Millisecond, "bootstrap did not become ready") {
		t.Fatalf("bootstrap did not become ready; last request error: %v", lastRequestErr)
	}
	defer closeImproveKandevResponseBody(t, ready)
	var result improvekandev.BootstrapResponse
	require.NoError(t, json.NewDecoder(ready.Body).Decode(&result))
	require.NotEmpty(t, result.WorkspaceID)
	require.NotEmpty(t, result.WorkflowID)
	require.NotEmpty(t, result.IssueWorkflowID)
}

func newImproveKandevTestRouterForUser(handler *improvekandev.Handler, userID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: userID})
		c.Next()
	})
	improvekandev.RegisterRoutes(router, handler)
	return router
}

func postImproveKandevBootstrap(t *testing.T, client *http.Client, baseURL string, asynchronous bool) (string, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"create_workspace": true, "async": asynchronous})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	require.NoError(t, err)
	defer closeImproveKandevResponseBody(t, response)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	if asynchronous {
		require.Equal(t, http.StatusAccepted, response.StatusCode, string(responseBody))
		var admitted struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.Unmarshal(responseBody, &admitted))
		require.NotEmpty(t, admitted.JobID)
		return admitted.JobID, responseBody
	}
	require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))
	return "", responseBody
}

func getImproveKandevBootstrapStatus(client *http.Client, baseURL, jobID string) (int, []byte, error) {
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/system/improve-kandev/bootstrap/"+jobID, nil)
	if err != nil {
		return 0, nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	body, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err != nil {
		return 0, nil, err
	}
	if closeErr != nil {
		return 0, nil, closeErr
	}
	return response.StatusCode, body, nil
}

func closeImproveKandevResponseBody(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}

func waitForImproveKandevBootstrapStatus(t *testing.T, client *http.Client, baseURL, jobID string, want int) []byte {
	t.Helper()
	var body []byte
	var lastRequestErr error
	if !assert.Eventually(t, func() bool {
		status, responseBody, err := getImproveKandevBootstrapStatus(client, baseURL, jobID)
		if err != nil {
			lastRequestErr = err
			return false
		}
		lastRequestErr = nil
		body = responseBody
		return status == want
	}, 10*time.Second, 25*time.Millisecond, "bootstrap job did not reach status %d", want) {
		t.Fatalf("bootstrap job did not reach status %d; last request error: %v", want, lastRequestErr)
	}
	return body
}

func TestImproveKandevBootstrapRetryOwnerScopeAndRestartReuse(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	cloner := &failOnceImproveKandevCloner{path: repoPath}
	handler := improvekandev.NewHandler(ts.TaskSvc, cloner, nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = handler.Stop() })
	handler.SetManagedGitHubForkProber(immediateImproveKandevGitHub{})
	ownerServer := httptest.NewServer(newImproveKandevTestRouterForUser(handler, "test-user"))
	t.Cleanup(ownerServer.Close)
	otherServer := httptest.NewServer(newImproveKandevTestRouterForUser(handler, "other-user"))
	t.Cleanup(otherServer.Close)

	firstJob, _ := postImproveKandevBootstrap(t, ownerServer.Client(), ownerServer.URL, true)
	firstResult := waitForImproveKandevBootstrapStatus(t, ownerServer.Client(), ownerServer.URL, firstJob, http.StatusInternalServerError)
	require.Contains(t, string(firstResult), "failed to register kandev repository")
	otherStatus, _, err := getImproveKandevBootstrapStatus(otherServer.Client(), otherServer.URL, firstJob)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, otherStatus)

	secondJob, _ := postImproveKandevBootstrap(t, ownerServer.Client(), ownerServer.URL, true)
	secondResult := waitForImproveKandevBootstrapStatus(t, ownerServer.Client(), ownerServer.URL, secondJob, http.StatusOK)
	var successful improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(secondResult, &successful))
	require.EqualValues(t, 2, cloner.calls.Load())

	// A fresh handler has an empty job map but reuses the workspace, repository,
	// and workflows already provisioned by the completed attempt.
	_ = handler.Stop()
	reusedCloner := &unexpectedImproveKandevCloner{}
	restarted := improvekandev.NewHandler(ts.TaskSvc, reusedCloner, nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = restarted.Stop() })
	restarted.SetManagedGitHubForkProber(immediateImproveKandevGitHub{})
	restartedServer := httptest.NewServer(newImproveKandevTestRouterForUser(restarted, "test-user"))
	t.Cleanup(restartedServer.Close)
	restartedJob, _ := postImproveKandevBootstrap(t, restartedServer.Client(), restartedServer.URL, true)
	restartedResult := waitForImproveKandevBootstrapStatus(t, restartedServer.Client(), restartedServer.URL, restartedJob, http.StatusOK)
	var reused improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(restartedResult, &reused))
	require.Equal(t, successful.WorkspaceID, reused.WorkspaceID)
	require.Equal(t, successful.RepositoryID, reused.RepositoryID)
	require.Equal(t, successful.WorkflowID, reused.WorkflowID)
	require.Equal(t, successful.IssueWorkflowID, reused.IssueWorkflowID)
	require.NotEqual(t, successful.BundleDir, reused.BundleDir)
	require.Zero(t, reusedCloner.calls.Load())
}

func TestImproveKandevBootstrapOverlappingAttemptsConverge(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = handler.Stop() })
	handler.SetManagedGitHubForkProber(immediateImproveKandevGitHub{})
	server := httptest.NewServer(newImproveKandevTestRouter(handler))
	t.Cleanup(server.Close)
	_, initialBody := postImproveKandevBootstrap(t, server.Client(), server.URL, false)
	var initial improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(initialBody, &initial))

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	handler.SetManagedGitHubForkProber(pairedImproveKandevGitHub{started: started, release: release})
	firstJob, _ := postImproveKandevBootstrap(t, server.Client(), server.URL, true)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first overlapping attempt did not reach its probe")
	}
	secondJob, _ := postImproveKandevBootstrap(t, server.Client(), server.URL, true)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("second overlapping attempt did not reach its probe")
	}
	close(release)

	firstBody := waitForImproveKandevBootstrapStatus(t, server.Client(), server.URL, firstJob, http.StatusOK)
	secondBody := waitForImproveKandevBootstrapStatus(t, server.Client(), server.URL, secondJob, http.StatusOK)
	var first, second improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(firstBody, &first))
	require.NoError(t, json.Unmarshal(secondBody, &second))
	for _, result := range []improvekandev.BootstrapResponse{first, second} {
		require.Equal(t, initial.WorkspaceID, result.WorkspaceID)
		require.Equal(t, initial.RepositoryID, result.RepositoryID)
		require.Equal(t, initial.WorkflowID, result.WorkflowID)
		require.Equal(t, initial.IssueWorkflowID, result.IssueWorkflowID)
	}
	require.NotEqual(t, first.BundleDir, second.BundleDir)
}

func TestImproveKandevBootstrapColdOverlappingAttemptsConverge(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	githubStarted := make(chan struct{}, 2)
	githubRelease := make(chan struct{})
	cloneStarted := make(chan struct{}, 2)
	cloneRelease := make(chan struct{})
	handler := improvekandev.NewHandler(ts.TaskSvc,
		pairedImproveKandevCloner{path: repoPath, started: cloneStarted, release: cloneRelease},
		nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = handler.Stop() })
	handler.SetManagedGitHubForkProber(pairedImproveKandevGitHub{started: githubStarted, release: githubRelease})
	server := httptest.NewServer(newImproveKandevTestRouter(handler))
	t.Cleanup(server.Close)

	firstJob, _ := postImproveKandevBootstrap(t, server.Client(), server.URL, true)
	secondJob, _ := postImproveKandevBootstrap(t, server.Client(), server.URL, true)
	for range 2 {
		select {
		case <-githubStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("both cold attempts did not reach the gated GitHub probe")
		}
	}
	close(githubRelease)
	for range 2 {
		select {
		case <-cloneStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("both cold attempts did not pass the empty repository lookup")
		}
	}
	close(cloneRelease)

	firstBody := waitForImproveKandevBootstrapStatus(t, server.Client(), server.URL, firstJob, http.StatusOK)
	secondBody := waitForImproveKandevBootstrapStatus(t, server.Client(), server.URL, secondJob, http.StatusOK)
	var first, second improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(firstBody, &first))
	require.NoError(t, json.Unmarshal(secondBody, &second))
	require.NotEmpty(t, first.RepositoryID)
	require.Equal(t, first.WorkspaceID, second.WorkspaceID)
	require.Equal(t, first.RepositoryID, second.RepositoryID)
	require.Equal(t, first.WorkflowID, second.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, second.IssueWorkflowID)
	require.NotEqual(t, first.BundleDir, second.BundleDir)

	repositories, err := ts.TaskSvc.ListRepositories(context.Background(), first.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, repositories, 1)
	workflows, err := ts.TaskSvc.ListWorkflows(context.Background(), first.WorkspaceID, true)
	require.NoError(t, err)
	require.Len(t, workflows, 2)
}

func TestImproveKandevBootstrapShutdownCancelsActiveWorker(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	cloneStarted := make(chan struct{}, 1)
	cloneCanceled := make(chan struct{}, 1)
	cloneRelease := make(chan struct{})
	handler := improvekandev.NewHandler(ts.TaskSvc,
		gatedImproveKandevCloner{path: repoPath, started: cloneStarted, release: cloneRelease, canceled: cloneCanceled},
		nil, nil, "test", ts.Logger)
	t.Cleanup(func() { _ = handler.Stop() })
	handler.SetManagedGitHubForkProber(immediateImproveKandevGitHub{})
	server := httptest.NewServer(newImproveKandevTestRouter(handler))
	t.Cleanup(server.Close)
	jobID, _ := postImproveKandevBootstrap(t, server.Client(), server.URL, true)
	select {
	case <-cloneStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap worker did not reach the clone")
	}
	require.NoError(t, handler.Stop())
	select {
	case <-cloneCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("handler shutdown did not cancel the active clone")
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- handler.Stop() }()
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		close(cloneRelease)
		select {
		case <-stopDone:
		case <-time.After(5 * time.Second):
			t.Fatal("handler Stop remained blocked after releasing the gated clone")
		}
		t.Fatal("handler Stop did not cancel and join the active bootstrap worker")
	}
	status, _, err := getImproveKandevBootstrapStatus(server.Client(), server.URL, jobID)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, status)
}
