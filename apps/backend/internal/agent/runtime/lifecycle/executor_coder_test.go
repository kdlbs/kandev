package lifecycle

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCoderWorkspaceNameIsTaskScoped(t *testing.T) {
	got := coderWorkspaceName("Team Agents", "ABCDEF12-3456-7890")
	if got != "team-agents-abcdef12-345" {
		t.Fatalf("coderWorkspaceName() = %q", got)
	}
}

func TestCoderDialLive(t *testing.T) {
	workspace := os.Getenv("KANDEV_TEST_CODER_WORKSPACE")
	if workspace == "" {
		t.Skip("set KANDEV_TEST_CODER_WORKSPACE for live Coder transport proof")
	}
	target := &SSHTarget{Host: workspace, Port: 22, User: "coder", IdentitySource: SSHIdentitySourceCoder, CoderWorkspace: workspace}
	client, err := DialSSH(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if err := session.Run("true"); err != nil {
		t.Fatal(err)
	}
}

func TestCoderEnsureCreatesMissingWorkspaceAndWaits(t *testing.T) {
	var calls [][]string
	m := &CoderWorkspaceManager{run: func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "create", "ssh":
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", args[0])
		}
	}}
	req := &ExecutorCreateRequest{TaskID: "TASK-123", Metadata: map[string]interface{}{
		MetadataKeyCoderTemplate: "ubuntu-dev", MetadataKeyCoderWorkspacePrefix: "kd",
	}}
	got, err := m.Ensure(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got != "kd-task-123" {
		t.Fatalf("workspace = %q", got)
	}
	want := [][]string{
		{"coder", "list", "--output", "json", "--search", "owner:me name:kd-task-123"},
		{"coder", "create", "--yes", "--template", "ubuntu-dev", "--use-parameter-defaults", "kd-task-123"},
		{"coder", "ssh", "--wait", "yes", "kd-task-123", "--", "true"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestCoderEnsureStartsStoppedWorkspace(t *testing.T) {
	var commands []string
	m := &CoderWorkspaceManager{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		commands = append(commands, args[0])
		if args[0] == "list" {
			return []byte(`[{"name":"shared","latest_build":{"status":"stopped"}}]`), nil
		}
		return nil, nil
	}}
	req := &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeyCoderWorkspace: "shared"}}
	if _, err := m.Ensure(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "list,start,ssh" {
		t.Fatalf("commands = %v", commands)
	}
}

func TestCoderEnsureRequiresTemplateForMissingWorkspace(t *testing.T) {
	m := &CoderWorkspaceManager{run: func(context.Context, string, ...string) ([]byte, error) { return []byte(`[]`), nil }}
	_, err := m.Ensure(context.Background(), &ExecutorCreateRequest{TaskID: "task"})
	if err == nil || !strings.Contains(err.Error(), "template is required") {
		t.Fatalf("err = %v", err)
	}
}
