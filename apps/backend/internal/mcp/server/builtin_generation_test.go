package mcp

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func generationRequest(t *testing.T, router http.Handler, path, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+secret)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}

func TestBuiltinGenerationStaleCalls(t *testing.T) {
	log := newTestLogger(t)
	backend := &generationEffectBackend{testBackend: testBackend{response: map[string]interface{}{"executors": []interface{}{}}}}
	initial := packageTestProfile(p.PackageCore)
	host := NewWithProfile(backend, "session", "task", 10005, log, "", false, initial)
	next := packageTestProfile(p.PackageCore, p.PackageConfiguration)
	digest, err := p.BuiltinBindingDigest(next)
	require.NoError(t, err)
	scope := p.ProviderMCPScope{BindingID: "binding-one", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 2, ProfileDigest: digest}
	secret := strings.Repeat("a", 64)
	require.NoError(t, host.PrepareBuiltinGeneration(scope, secret, next))
	router := gin.New()
	host.RegisterRoutes(router)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"generation-test","version":"1"}}}`
	path := "/mcp/generations/binding-one/packages/configuration"
	initialized := generationRequest(t, router, path, secret, initialize)
	require.Equal(t, 200, initialized.Code)
	require.Contains(t, initialized.Body.String(), "binding-one", "fresh MCP instructions must identify their binding")
	require.Equal(t, 401, generationRequest(t, router, path, "wrong", initialize).Code)
	child := host.builtinGenerationServer("binding-one")
	require.NotNil(t, child)
	// Verifying permits discovery, but local and backend tools both stay closed.
	result := callTool(t, child, "list_executors_kandev", map[string]interface{}{})
	require.True(t, result.IsError)
	require.Equal(t, 0, backend.effects)
	require.NoError(t, host.RetireBuiltinGeneration(""))
	require.NoError(t, host.AcceptBuiltinGeneration("binding-one"))
	result = callTool(t, child, "list_executors_kandev", map[string]interface{}{})
	require.False(t, result.IsError)
	require.Equal(t, 1, backend.effects)
	require.Equal(t, scope, backend.scope)
	require.Equal(t, 410, generationRequest(t, router, "/mcp", secret, initialize).Code)
	require.NoError(t, host.RetireBuiltinGeneration("binding-one"))
	result = callTool(t, child, "list_executors_kandev", map[string]interface{}{})
	require.True(t, result.IsError)
	require.Equal(t, 1, backend.effects)
	require.Equal(t, 410, generationRequest(t, router, path, secret, initialize).Code)
	require.Error(t, host.AcceptBuiltinGeneration("binding-one"), "retired scopes cannot regain authority")
}

func TestBuiltinGenerationLegacyRoutes(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	host := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(p.PackageCore))
	release, err := host.beginGenerationEffect(context.Background())
	require.NoError(t, err)
	require.Error(t, host.RetireBuiltinGeneration(""), "an admitted effect must settle first")
	release()
	require.NoError(t, host.RetireBuiltinGeneration(""))
	_, err = host.beginGenerationEffect(context.Background())
	require.Error(t, err)
}

type generationEffectBackend struct {
	testBackend
	effects int
	scope   p.ProviderMCPScope
}

func (b *generationEffectBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	b.effects++
	b.scope, _ = streams.ProviderMCPScopeFromContext(ctx)
	return b.testBackend.RequestPayload(ctx, action, payload, result)
}

func TestBuiltinGenerationPreparationCanReleaseWithoutRetirement(t *testing.T) {
	host := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(p.PackageCore))
	require.NoError(t, host.FreezeBuiltinGeneration(""))
	_, err := host.beginGenerationEffect(context.Background())
	require.Error(t, err)
	require.NoError(t, host.ResumeBuiltinGeneration(""))
	release, err := host.beginGenerationEffect(context.Background())
	require.NoError(t, err)
	require.Error(t, host.FreezeBuiltinGeneration(""), "preparation must not cancel an admitted effect")
	release()
	require.NoError(t, host.FreezeBuiltinGeneration(""))
	require.NoError(t, host.RetireBuiltinGeneration(""))
	require.Error(t, host.ResumeBuiltinGeneration(""), "a committed retirement cannot expire back into old authority")
	require.NoError(t, host.Close(context.Background()))
}

func TestBuiltinGenerationRejectsForeignWorkspace(t *testing.T) {
	initial := packageTestProfile(p.PackageCore)
	host := NewWithProfile(NewChannelBackendClient(newTestLogger(t)), "session", "task", 10005, newTestLogger(t), "", false, initial)
	foreign := packageTestProfile(p.PackageCore)
	foreign.BuiltinPackages.WorkspaceID = "foreign"
	digest, err := p.BuiltinBindingDigest(foreign)
	require.NoError(t, err)
	scope := p.ProviderMCPScope{BindingID: "foreign", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}
	require.Error(t, host.PrepareBuiltinGeneration(scope, strings.Repeat("s", 64), foreign))
}
