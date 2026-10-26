//go:build linux

package sandboxruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SANDBOX-003.1 AC-AGENTS-SANDBOX-003.3 AC-AGENTS-SANDBOX-003.5
func TestSandboxRuntimeRealApprovalAndPrivateBoundary(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	pluginService, policies, selection := runtimeFixture(t, ctx, packagePath)
	policies.SetSessionAuthorizer(func(_ context.Context, sessionID string) error {
		if sessionID != "approval-session" {
			return policy.ErrNotFound
		}
		return nil
	})
	selection.WorkspaceID, selection.AgentProfileID = "workspace", "agent-profile"
	selection.TaskID = "sandbox-task"
	policies.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "workspace" {
			return policy.ErrAuthority
		}
		return nil
	})
	work := t.TempDir()
	outside := filepath.Join(t.TempDir(), "approval.txt")
	require.NoError(t, os.WriteFile(outside, []byte("operator-selected"), 0600))
	child := buildApprovalChild(t, ctx)
	providers, err := pluginService.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	binding := providers[0].Binding
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer destination.Close()
	destinationURL, err := url.Parse(destination.URL)
	require.NoError(t, err)
	destinationPort, err := strconv.Atoi(destinationURL.Port())
	require.NoError(t, err)
	grants := contract.Grants{KandevMCP: true, Network: []contract.NetworkGrant{{Host: destinationURL.Hostname(), Port: uint16(destinationPort), Protocol: "tcp"}}, Filesystem: []contract.FilesystemGrant{{Reference: "agent_state", Writable: true}, {Path: work, Writable: true}, {Path: child}}}
	selected := policy.Policy{ID: "selected", Name: "Work", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: grants}
	revision, err := policies.Publish(ctx, selected, 0)
	require.NoError(t, err)
	selected.ID = "ceiling"
	selected.Name = "Ceiling"
	selected.Grants.Filesystem = append(selected.Grants.Filesystem, contract.FilesystemGrant{Path: outside})
	ceiling, err := policies.Publish(ctx, selected, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: ceiling.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, pluginService, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	forward, calls := runtimeMCPFixture(t)
	prepared, err := manager.Prepare(ctx, PrepareRequest{MCPForward: forward, Selection: selection, SessionID: "approval-session", ExecutionID: "approval-execution", Generation: 1, WorkDir: work, Arguments: []string{child}})
	require.NoError(t, err)
	require.Equal(t, prepared.agentState.Directories().Home, prepared.Launch().ChildHome)
	args, env, err := contract.PrepareLaunch(prepared.Launch(), []string{child}, []string{"UNAPPROVED_SECRET=private-parent-value"})
	require.NoError(t, err)
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Env = env
	command.Dir = work
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, prepared.MarkStart(ctx))
	containmentLease, err := prepared.Launch().Containment.Configure(command)
	require.NoError(t, err)
	require.NoError(t, command.Start())
	require.NoError(t, containmentLease.Close())
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			_ = command.Wait()
		}
	})
	owner, err := contract.CaptureProcessOwner(prepared.Launch(), command.Process.Pid)
	require.NoError(t, err)
	reader := bufio.NewReader(output)
	ready, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", ready)
	require.NoError(t, prepared.BindOwner(ctx, "runtime", owner))
	send := func(op, target string) {
		t.Helper()
		raw, e := json.Marshal(map[string]string{"operation": op, "target": target})
		require.NoError(t, e)
		_, e = input.Write(append(raw, '\n'))
		require.NoError(t, e)
	}
	result := func(want bool) {
		t.Helper()
		line, e := reader.ReadString('\n')
		require.NoError(t, e)
		require.Equal(t, strconv.FormatBool(want)+"\n", line)
	}
	send("write", filepath.Join(work, "allowed"))
	result(true)
	send("state", "")
	result(true)
	require.Error(t, manager.CleanupSessionState(ctx, selection.TaskID, "approval-session"))
	require.FileExists(t, filepath.Join(prepared.Launch().ChildHome, "conversation"))
	send("env", "UNAPPROVED_SECRET")
	result(false)
	for _, decision := range []string{"allow_once", "deny"} {
		send("read", outside)
		var request policy.ApprovalView
		require.Eventually(t, func() bool {
			approvals, e := policies.ListApprovals(ctx, "approval-session")
			if e != nil {
				return false
			}
			for _, approval := range approvals {
				if approval.State == policy.ApprovalPending {
					request = approval
					return true
				}
			}
			return false
		}, 3*time.Second, 10*time.Millisecond)
		require.True(t, request.CanDecide)
		_, err = policies.DecideApproval(ctx, request.AttemptID, request.RequestID, request.Version, decision)
		require.NoError(t, err)
		result(decision == "allow_once")
	}
	for _, target := range []string{prepared.launch.Prefix[4], fmt.Sprintf("/proc/%d/environ", command.Process.Pid), fmt.Sprintf("/proc/%d/fd/0", command.Process.Pid)} {
		send("read", target)
		result(false)
	}
	callback, err := url.Parse(prepared.channel.URL())
	require.NoError(t, err)
	send("connect", callback.Host)
	result(false)
	send("proxy", destinationURL.Host)
	result(true)
	send("proxy", callback.Host)
	result(false)
	send("mcp", prepared.Launch().MCPURL)
	result(true)
	require.EqualValues(t, 1, calls.Load())
	send("mcp", "http://"+prepared.mcp.Address()+"/api/v1/settings")
	result(false)
	require.EqualValues(t, 1, calls.Load())
	send("read", filepath.Join(t.TempDir(), "outside-ceiling"))
	result(false)
	_, err = io.WriteString(input, "exit\n")
	require.NoError(t, err)
	require.NoError(t, input.Close())
	require.NoError(t, command.Wait())
	report, err := manager.RefreshReport(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	assertRuntimeDenials(t, report, outside, false)
	assertRuntimeReportSurvivesInvalidAudit(t, ctx, prepared, report)
	require.NoError(t, prepared.Close())
	report, err = policies.ReadReport(ctx, "approval-session", prepared.Launch().AttemptID)
	require.NoError(t, err)
	assertRuntimeDenials(t, report, outside, true)
	assertRuntimeLearning(t, ctx, manager, policies, report, outside)
	// A new attempt for the same agent session must retain its explicit state grant.
	restarted, err := manager.Prepare(ctx, PrepareRequest{RestartOf: prepared.Launch().AttemptID, MCPForward: forward, Selection: selection, SessionID: "approval-session", ExecutionID: "restart-execution", Generation: 1, WorkDir: work, Arguments: []string{child}})
	require.NoError(t, err)
	stored, err := policies.ReadAttempt(ctx, "org", restarted.Launch().AttemptID)
	require.NoError(t, err)
	var found bool
	for _, grant := range stored.Snapshot.Grants.Filesystem {
		if content, readErr := os.ReadFile(filepath.Join(grant.Path, "conversation")); readErr == nil {
			require.Equal(t, "retained", string(content))
			found = true
		}
	}
	require.True(t, found)
	require.NoError(t, policies.Revoke(ctx, revision.ID, revision.Digest))
	require.Error(t, restarted.MarkStart(ctx), "revocation after preparation must prevent dispatch")
	require.NoError(t, restarted.Close())
	after, err := policies.GetRevision(ctx, revision.ID)
	require.NoError(t, err)
	require.Equal(t, revision.Digest, after.Digest, "allow once must not publish or expand policy")
	require.NoError(t, manager.CleanupSessionState(ctx, selection.TaskID, "approval-session"))
	require.NoDirExists(t, filepath.Dir(restarted.Launch().ChildHome))
}

func buildApprovalChild(t *testing.T, ctx context.Context) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "child.go")
	binary := filepath.Join(root, "child")
	code := `package main
import("bufio";"encoding/json";"encoding/base64";"fmt";"net";"net/http";"net/url";"os";"time";"strings";"io")
func main(){fmt.Println("ready");scanner:=bufio.NewScanner(os.Stdin);for scanner.Scan(){if scanner.Text()=="exit"{return};var op map[string]string;if json.Unmarshal(scanner.Bytes(),&op)!=nil{os.Exit(2)};ok:=false;switch op["operation"]{case "read":_,err:=os.ReadFile(op["target"]);ok=err==nil;case "write":ok=os.WriteFile(op["target"],[]byte("ok"),0600)==nil;case "state":ok=state();case "env":ok=os.Getenv(op["target"])!="";case "proxy":ok=proxy(op["target"]);case "mcp":ok=mcp(op["target"]);case "connect":connection,err:=net.DialTimeout("tcp",op["target"],500*time.Millisecond);ok=err==nil;if connection!=nil{_=connection.Close()}};fmt.Println(ok)}}
func state()bool{home:=os.Getenv("HOME");if home==""{return false};for _,key:=range []string{"XDG_CONFIG_HOME","XDG_STATE_HOME","TMPDIR"}{dir:=os.Getenv(key);if !strings.HasPrefix(dir,home+"/")||os.WriteFile(dir+"/owned",[]byte("session"),0600)!=nil{return false}};return os.WriteFile(home+"/conversation",[]byte("retained"),0600)==nil}
func proxy(target string)bool{address,err:=url.Parse(os.Getenv("HTTP_PROXY"));if err!=nil{return false};connection,err:=net.DialTimeout("tcp",address.Host,time.Second);if err!=nil{return false};defer connection.Close();_=connection.SetDeadline(time.Now().Add(time.Second));auth:="";if address.User!=nil{password,_:=address.User.Password();auth="Proxy-Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte(address.User.Username()+":"+password))+"\r\n"};_,err=fmt.Fprintf(connection,"CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n",target,target,auth);if err!=nil{return false};response,err:=http.ReadResponse(bufio.NewReader(connection),&http.Request{Method:"CONNECT"});return err==nil && response.StatusCode==200}
func mcp(target string)bool{proxyURL,err:=url.Parse(os.Getenv("HTTP_PROXY"));if err!=nil{return false};transport:=&http.Transport{Proxy:http.ProxyURL(proxyURL)};defer transport.CloseIdleConnections();client:=&http.Client{Transport:transport,Timeout:4*time.Second};session:="";bodies:=[]string{` + "`" + `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"sandbox-test","version":"1"}}}` + "`" + `,` + "`" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_task_plan_kandev","arguments":{}}}` + "`" + `};for index,body:=range bodies{request,err:=http.NewRequest("POST",target,strings.NewReader(body));if err!=nil{return false};request.Header.Set("Content-Type","application/json");request.Header.Set("Accept","application/json, text/event-stream");if session!=""{request.Header.Set("Mcp-Session-Id",session)};response,err:=client.Do(request);if err!=nil{fmt.Fprintln(os.Stderr,"MCP transport failed",index);return false};content,readErr:=io.ReadAll(response.Body);_=response.Body.Close();if readErr!=nil||response.StatusCode!=200{fmt.Fprintln(os.Stderr,"MCP response failed",index,response.StatusCode);return false};if index==0{session=response.Header.Get("Mcp-Session-Id")}else if !strings.Contains(string(content),"sandbox-mcp-permitted"){fmt.Fprintln(os.Stderr,"MCP content mismatch");return false}};return true}`
	require.NoError(t, os.WriteFile(source, []byte(code), 0600))
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
	return binary
}
