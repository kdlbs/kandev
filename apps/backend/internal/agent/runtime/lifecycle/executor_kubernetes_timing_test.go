package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestKubernetesLaunchTimingSuccess(t *testing.T) {
	executor, logs := observedKubernetesExecutor(t)
	executor.launchTimingClock = sequenceKubernetesTimingClock(
		time.Date(2026, time.September, 9, 10, 0, 0, 0, time.UTC),
		10,
		10*time.Millisecond,
	)

	instance, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())

	require.NoError(t, err)
	require.NotNil(t, instance)
	stages := kubernetesTimingEntries(logs, "kubernetes.launch.stage")
	require.Len(t, stages, 4)
	require.Equal(t, []string{"storage", "pod_ready", "bootstrap", "agentctl_connect"},
		[]string{
			stages[0].ContextMap()["stage"].(string),
			stages[1].ContextMap()["stage"].(string),
			stages[2].ContextMap()["stage"].(string),
			stages[3].ContextMap()["stage"].(string),
		})
	for _, entry := range stages {
		fields := entry.ContextMap()
		require.Equal(t, "success", fields["outcome"])
		require.EqualValues(t, 10, fields["duration_ms"])
		require.Equal(t, "fresh", fields["mode"])
	}
	completed := kubernetesTimingEntries(logs, "kubernetes.launch.completed")
	require.Len(t, completed, 1)
	require.Equal(t, "success", completed[0].ContextMap()["outcome"])
	require.EqualValues(t, 90, completed[0].ContextMap()["duration_ms"])
	require.NotEmpty(t, completed[0].ContextMap()["attempt_id"])
	require.Equal(t, "instance-1", completed[0].ContextMap()["instance_id"])
	require.Equal(t, "task-1", completed[0].ContextMap()["task_id"])
	require.Equal(t, "session-1", completed[0].ContextMap()["session_id"])
}

func TestKubernetesLaunchTimingFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configure   func(*ExecutorCreateRequest, *fakeKubernetesResources, *recordingKubernetesExec)
		failedStage string
		outcome     string
	}{
		{
			name: "storage",
			configure: func(req *ExecutorCreateRequest, resources *fakeKubernetesResources, _ *recordingKubernetesExec) {
				setManagedKubernetesWorkspace(req)
				resources.createPVCBeforeCommitErr = errors.New("storage failed")
			},
			failedStage: "storage",
			outcome:     "error",
		},
		{
			name: "pod readiness",
			configure: func(_ *ExecutorCreateRequest, resources *fakeKubernetesResources, _ *recordingKubernetesExec) {
				resources.waitForPodRunning = func(context.Context, string, string, string) (*corev1.Pod, error) {
					return nil, errors.New("pod readiness failed")
				}
			},
			failedStage: "pod_ready",
			outcome:     "error",
		},
		{
			name: "bootstrap cancellation",
			configure: func(_ *ExecutorCreateRequest, _ *fakeKubernetesResources, execs *recordingKubernetesExec) {
				execs.err = context.Canceled
			},
			failedStage: "bootstrap",
			outcome:     "canceled",
		},
		{
			name:        "agentctl connection",
			configure:   func(_ *ExecutorCreateRequest, _ *fakeKubernetesResources, _ *recordingKubernetesExec) {},
			failedStage: "agentctl_connect",
			outcome:     "error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resources := &fakeKubernetesResources{}
			execs := &recordingKubernetesExec{}
			executor, logs := observedKubernetesExecutorWith(
				t, resources, execs, &recordingKubernetesForwarder{localPorts: map[uint16]uint16{}},
			)
			req := validKubernetesCreateRequest()
			tt.configure(req, resources, execs)

			_, err := executor.CreateInstance(context.Background(), req)

			require.Error(t, err)
			stages := kubernetesTimingEntries(logs, "kubernetes.launch.stage")
			require.NotEmpty(t, stages)
			lastStage := stages[len(stages)-1].ContextMap()
			require.Equal(t, tt.failedStage, lastStage["stage"])
			require.Equal(t, tt.outcome, lastStage["outcome"])
			completed := kubernetesTimingEntries(logs, "kubernetes.launch.completed")
			require.Len(t, completed, 1)
			require.Equal(t, tt.outcome, completed[0].ContextMap()["outcome"])
			require.Equal(t, tt.failedStage, completed[0].ContextMap()["failed_stage"])
		})
	}
}

func TestKubernetesLaunchTimingFinalization(t *testing.T) {
	executor, logs := observedKubernetesExecutor(t)
	finalizationErr := errors.New("finalization contained sentinel-secret")
	req := validKubernetesCreateRequest()
	req.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if getMetadataString(metadata, MetadataKeyKubernetesInventoryState) ==
			KubernetesInventoryStateReady {
			return finalizationErr
		}
		return nil
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorIs(t, err, finalizationErr)
	stages := kubernetesTimingEntries(logs, "kubernetes.launch.stage")
	require.Len(t, stages, 4)
	completed := kubernetesTimingEntries(logs, "kubernetes.launch.completed")
	require.Len(t, completed, 1)
	require.Equal(t, "finalize", completed[0].ContextMap()["failure_site"])
	require.NotContains(t, strings.Join(logMessages(logs), "\n"), "sentinel-secret")
}

func TestKubernetesLaunchTimingConcurrentAttempts(t *testing.T) {
	timingOne := newKubernetesLaunchTiming(nil, &ExecutorCreateRequest{
		InstanceID: "instance-one", TaskID: "task-one", SessionID: "session-one",
	})
	timingTwo := newKubernetesLaunchTiming(nil, &ExecutorCreateRequest{
		InstanceID: "instance-two", TaskID: "task-two", SessionID: "session-two",
	})
	var wait sync.WaitGroup
	for _, timing := range []*kubernetesLaunchTiming{timingOne, timingTwo} {
		wait.Add(1)
		go func(timing *kubernetesLaunchTiming) {
			defer wait.Done()
			timing.recordStage("storage", time.Now(), nil)
			timing.complete(nil)
		}(timing)
	}
	wait.Wait()
	require.NotEqual(t, timingOne.attemptID, timingTwo.attemptID)
	require.Equal(t, "instance-one", timingOne.instanceID)
	require.Equal(t, "instance-two", timingTwo.instanceID)
}

func observedKubernetesExecutor(t *testing.T) (*KubernetesExecutor, *observer.ObservedLogs) {
	t.Helper()
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	return observedKubernetesExecutorWith(
		t,
		&fakeKubernetesResources{},
		&recordingKubernetesExec{},
		&recordingKubernetesForwarder{localPorts: map[uint16]uint16{
			uint16(8765): controlPort,
			41001:        instancePort,
		}},
	)
}

func observedKubernetesExecutorWith(
	t *testing.T,
	resources *fakeKubernetesResources,
	execs *recordingKubernetesExec,
	forwards *recordingKubernetesForwarder,
) (*KubernetesExecutor, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	executor := newFakeKubernetesExecutorWithForwarder(resources, execs, forwards)
	executor.logger = log
	executor.healthRetryDelay = time.Nanosecond
	return executor, logs
}

func sequenceKubernetesTimingClock(start time.Time, calls int, step time.Duration) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= calls {
			return start.Add(time.Duration(calls) * step)
		}
		current := start.Add(time.Duration(index) * step)
		index++
		return current
	}
}

func kubernetesTimingEntries(logs *observer.ObservedLogs, message string) []observer.LoggedEntry {
	return logs.FilterMessage(message).All()
}

func logMessages(logs *observer.ObservedLogs) []string {
	entries := logs.All()
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	return messages
}
