package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestPluginExecutorFixtureContract(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	profile := &pluginsdk.ExecutorProfileSnapshot{
		ProfileId:    "profile-1",
		Config:       map[string]string{"region": "eu-west-1"},
		SecretValues: map[string]string{"credential": "fixture-secret"},
	}
	validated, err := plugin.ValidateExecutorProfile(context.Background(), &pluginsdk.ValidateExecutorProfileRequest{Profile: profile})
	require.NoError(t, err)
	require.Empty(t, validated.GetFieldErrors())
	require.Equal(t, "bounded", validated.GetCapabilities().GetRetention())
	require.EqualValues(t, 8*60*60, validated.GetCapabilities().GetMaximumLifetimeSeconds())

	invalid, err := plugin.ValidateExecutorProfile(context.Background(), &pluginsdk.ValidateExecutorProfileRequest{
		Profile: &pluginsdk.ExecutorProfileSnapshot{},
	})
	require.NoError(t, err)
	require.Len(t, invalid.GetFieldErrors(), 1)
	require.Equal(t, "region", invalid.GetFieldErrors()[0].GetField())

	ctx := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: executorProviderKey,
		ContractVersion: 1, EnvironmentId: "environment-1", OperationId: "operation-1", InputDigest: "digest-1",
	}
	provisioned, err := plugin.ProvisionExecutorEnvironment(context.Background(), &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "fixture:environment-1", provisioned.GetResource().GetResourceHandle())
	require.Equal(t, "bounded", provisioned.GetResource().GetRetention())
	require.NotContains(t, provisioned.GetResource().GetStateJson(), "fixture-secret")

	recovered, err := plugin.RecoverExecutorOperation(context.Background(), &pluginsdk.RecoverExecutorOperationRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "found", recovered.GetOutcome())
	require.Equal(t, provisioned.GetResource().GetResourceHandle(), recovered.GetResource().GetResourceHandle())

	attached, err := plugin.AttachExecutorEnvironment(context.Background(), &pluginsdk.AttachExecutorEnvironmentRequest{Resource: provisioned.GetResource()})
	require.NoError(t, err)
	require.Equal(t, provisioned.GetResource().GetResourceHandle(), attached.GetResource().GetResourceHandle())

	inspected, err := plugin.InspectExecutorEnvironment(context.Background(), &pluginsdk.InspectExecutorEnvironmentRequest{Resource: provisioned.GetResource()})
	require.NoError(t, err)
	require.Equal(t, "running", inspected.GetState())

	connectionRequest := &pluginsdk.ResolveExecutorConnectionRequest{
		Context: ctx, Resource: provisioned.GetResource(), Purpose: "agentctl", RuntimePort: 8765,
	}
	connection, err := plugin.ResolveExecutorConnection(context.Background(), connectionRequest)
	require.NoError(t, err)
	require.NotNil(t, connection.GetLease())
	require.NotContains(t, connection.GetLease().GetBaseUrl(), "fixture-secret")
	require.NotContains(t, connection.GetLease().GetHttpHeaders()["X-Fixture-Token"], "fixture-secret")

	response := fixtureHTTPSRequest(t, plugin, connection.GetLease(), "fixture-request-one")
	require.Equal(t, "fixture-request-one", response)

	instanceLease, err := plugin.ResolveExecutorConnection(context.Background(), &pluginsdk.ResolveExecutorConnectionRequest{
		Context: ctx, Resource: provisioned.GetResource(), Purpose: "agentctl", RuntimePort: 41001,
	})
	require.NoError(t, err)
	require.NotNil(t, instanceLease.GetLease(), "agentctl instance ports must be leasable")

	rotated, err := plugin.ResolveExecutorConnection(context.Background(), connectionRequest)
	require.NoError(t, err)
	require.NotEqual(t, connection.GetLease().GetGeneration(), rotated.GetLease().GetGeneration())
	require.NotEqual(t, connection.GetLease().GetHttpHeaders()["X-Fixture-Token"], rotated.GetLease().GetHttpHeaders()["X-Fixture-Token"])
	staleResponse, err := fixtureHTTPSClient(t, plugin, connection.GetLease()).Get(connection.GetLease().GetBaseUrl() + "/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, staleResponse.StatusCode)
	require.NoError(t, staleResponse.Body.Close())
	require.Equal(t, "fixture-request-two", fixtureHTTPSRequest(t, plugin, rotated.GetLease(), "fixture-request-two"))

	var leaseState map[string]any
	data, err := os.ReadFile(filepath.Join(plugin.dataDir, fixtureExecutorStateFileName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &leaseState))
	require.NotContains(t, string(data), "fixture-secret")

	destroyed, err := plugin.DestroyExecutorEnvironment(context.Background(), &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: ctx, Resource: provisioned.GetResource(),
	})
	require.NoError(t, err)
	require.True(t, destroyed.GetConfirmedAbsent())
}

func TestPluginExecutorFixtureRecoversLostProvisionReplyWithoutAllocatingAgain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(fixtureExecutorBarrierEnv, fixtureBarrierLoseProvisionReplyOnce)
	profile := &pluginsdk.ExecutorProfileSnapshot{ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"}}
	ctx := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: executorProviderKey,
		ContractVersion: 1, EnvironmentId: "environment-1", OperationId: "operation-1", InputDigest: "digest-1",
	}

	first := newFixturePluginAt(dir)
	_, err := first.ProvisionExecutorEnvironment(context.Background(), &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile})
	require.ErrorContains(t, err, "fixture provision reply lost")
	require.NoError(t, first.Close())

	restarted := newFixturePluginAt(dir)
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	recovered, err := restarted.RecoverExecutorOperation(context.Background(), &pluginsdk.RecoverExecutorOperationRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "found", recovered.GetOutcome())
	require.Equal(t, "fixture:environment-1", recovered.GetResource().GetResourceHandle())
	require.Equal(t, 1, restarted.executorAllocationCount())

	provisioned, err := restarted.ProvisionExecutorEnvironment(context.Background(), &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, recovered.GetResource().GetResourceHandle(), provisioned.GetResource().GetResourceHandle())
	require.Equal(t, 1, restarted.executorAllocationCount())
}

func TestPluginExecutorFixtureReportsAbsentForUnknownOperation(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	ctx := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: executorProviderKey,
		ContractVersion: 1, EnvironmentId: "environment-never-allocated", OperationId: "operation-never-allocated", InputDigest: "digest-1",
	}

	recovered, err := plugin.RecoverExecutorOperation(context.Background(), &pluginsdk.RecoverExecutorOperationRequest{
		Context: ctx, Profile: &pluginsdk.ExecutorProfileSnapshot{ProfileId: "profile-1"},
	})
	require.NoError(t, err)
	require.Equal(t, "absent", recovered.GetOutcome())
}

func TestPluginExecutorFixtureFailureBarriersAreRetryable(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	ctx := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: executorProviderKey,
		ContractVersion: 1, EnvironmentId: "environment-1", OperationId: "operation-1", InputDigest: "digest-1",
	}
	profile := &pluginsdk.ExecutorProfileSnapshot{ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"}}
	provisioned, err := plugin.ProvisionExecutorEnvironment(context.Background(), &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)

	t.Setenv(fixtureExecutorBarrierEnv, fixtureBarrierUnavailableLeaseOnce)
	request := &pluginsdk.ResolveExecutorConnectionRequest{
		Context: ctx, Resource: provisioned.GetResource(), Purpose: "agentctl", RuntimePort: 8765,
	}
	blocked, err := plugin.ResolveExecutorConnection(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "connection_unavailable", blocked.GetError().GetCode())

	retry, err := plugin.ResolveExecutorConnection(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, retry.GetLease())

	t.Setenv(fixtureExecutorBarrierEnv, fixtureBarrierUnconfirmedCleanupOnce)
	delayed, err := plugin.DestroyExecutorEnvironment(context.Background(), &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: ctx, Resource: provisioned.GetResource(),
	})
	require.NoError(t, err)
	require.False(t, delayed.GetConfirmedAbsent())
	require.Equal(t, "cleanup_unconfirmed", delayed.GetError().GetCode())

	cleaned, err := plugin.DestroyExecutorEnvironment(context.Background(), &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: ctx, Resource: provisioned.GetResource(),
	})
	require.NoError(t, err)
	require.True(t, cleaned.GetConfirmedAbsent())
}

func fixtureHTTPSClient(t *testing.T, plugin *fixturePlugin, lease *pluginsdk.ExecutorConnectionLease) *http.Client {
	t.Helper()
	certificates, err := x509.SystemCertPool()
	if err != nil || certificates == nil {
		certificates = x509.NewCertPool()
	}
	certificates.AddCert(plugin.certificateForLease(lease))
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certificates, MinVersion: tls.VersionTLS12}}}
}

func fixtureHTTPSRequest(t *testing.T, plugin *fixturePlugin, lease *pluginsdk.ExecutorConnectionLease, value string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, lease.GetBaseUrl()+"/echo?value="+value, nil)
	require.NoError(t, err)
	for name, value := range lease.GetHttpHeaders() {
		request.Header.Set(name, value)
	}
	response, err := fixtureHTTPSClient(t, plugin, lease).Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return string(body)
}
