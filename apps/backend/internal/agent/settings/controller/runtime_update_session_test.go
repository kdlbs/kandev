package controller

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestRuntimeFixtureProcess(t *testing.T) {
	if os.Getenv("KANDEV_RUNTIME_PROCESS_FIXTURE") != "1" {
		return
	}
	version := os.Args[len(os.Args)-1]
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println(version)
	}
	os.Exit(0)
}

type isolatedRuntimeUpdater struct {
	*recoveryRuntimeUpdater
	directory   string
	packageName string
}

func (u *isolatedRuntimeUpdater) version(command agents.Command) string {
	for _, arg := range command.Args() {
		arg = strings.TrimPrefix(arg, "--package=")
		if version, found := strings.CutPrefix(arg, u.packageName+"@"); found {
			return version
		}
	}
	return ""
}

func (u *isolatedRuntimeUpdater) RunUpdate(ctx context.Context, command agents.Command, chunk func(string)) error {
	if err := u.recoveryRuntimeUpdater.RunUpdate(ctx, command, chunk); err != nil {
		return err
	}
	version := u.version(command)
	return os.WriteFile(filepath.Join(u.directory, version), []byte(version), 0o600)
}

func (u *isolatedRuntimeUpdater) Probe(_ context.Context, _ string, command agents.Command) (hostutility.AgentCapabilities, error) {
	version := u.version(command)
	content, err := os.ReadFile(filepath.Join(u.directory, version))
	if err != nil {
		return hostutility.AgentCapabilities{}, err
	}
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: string(content)}, nil
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.4, AC-AGENTS-RUNTIME-NOTIFY-002.5
func TestLiveRuntimeProcessSurvivesAutomaticActivationRollbackAndDefault(t *testing.T) {
	spec := agents.NewGemini().ManagedNPMRuntime()
	previous := spec.DefaultVersionOrPinned()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeFixtureProcess$", "--", previous)
	process.Env = append(os.Environ(), "KANDEV_RUNTIME_PROCESS_FIXTURE=1")
	input, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = process.Wait() })
	reader := bufio.NewReader(output)
	assertLive := func() {
		t.Helper()
		if _, err := fmt.Fprintln(input, "version"); err != nil {
			t.Fatal(err)
		}
		version, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(version) != previous {
			t.Fatalf("live process changed: %q,%v", version, err)
		}
	}
	assertLive()
	selection := newRecoverySelectionStore()
	updater := &isolatedRuntimeUpdater{recoveryRuntimeUpdater: &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", "8.0.0", previous}}}, directory: t.TempDir(), packageName: spec.Package}
	c, hub := autoController(t, updater, selection, &autoMemorySettings{})
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	assertLive()
	_, effective, _, err := c.runtimeVersions(ctx, "gemini", spec)
	if err != nil || effective != "9.0.0" {
		t.Fatalf("future selection: %s,%v", effective, err)
	}
	rollback, err := c.EnqueueAgentUpdate(ctx, "gemini", "8.0.0")
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, rollback.JobID, dto.AgentUpdateJobStatusSucceeded)
	assertLive()
	_, effective, _, err = c.runtimeVersions(ctx, "gemini", spec)
	if err != nil || effective != "8.0.0" {
		t.Fatalf("rollback selection: %s,%v", effective, err)
	}
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+spec.Package)
	if err != nil || policy.Enabled {
		t.Fatalf("manual rollback did not withdraw automation: %+v,%v", policy, err)
	}
	reset, err := c.EnqueueAgentUpdateUseDefault(ctx, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, reset.JobID, dto.AgentUpdateJobStatusSucceeded)
	assertLive()
	_, effective, _, err = c.runtimeVersions(ctx, "gemini", spec)
	if err != nil || effective != previous {
		t.Fatalf("reviewed default selection: %s,%v", effective, err)
	}
}
