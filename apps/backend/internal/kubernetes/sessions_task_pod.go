package kubernetes

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
)

func isTaskPodInventory(metadata map[string]interface{}) bool {
	shared, _ := metadata[agentkubernetes.MetadataKeyTaskOwned].(bool)
	return shared
}

// Resolve physical inventory only after authorizing the session's owning task.
func (h *Handler) canonicalTaskPodInventory(ctx context.Context, run *models.ExecutorRunning, session *models.TaskSession) (*models.ExecutorRunning, error) {
	if !isTaskPodInventory(run.Metadata) {
		return run, nil
	}
	store, ok := h.repo.(interface {
		GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error)
	})
	if !ok {
		return run, errors.New("kubernetes environment store unavailable")
	}
	record, err := store.GetKubernetesEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return run, err
	}
	if record == nil || record.TaskID != run.TaskID || record.EnvironmentID != session.TaskEnvironmentID || metadataString(record.Metadata, metadataResourceEnv) != record.EnvironmentID {
		return run, models.ErrWorkspaceReuseUnsafe
	}
	copyRun := *run
	copyRun.Metadata = record.Metadata
	return &copyRun, nil
}

// Retained physical resources stay visible when their last session is removed.
func (h *Handler) appendRetainedTaskPods(ctx context.Context, client kubeclient.Interface, executorID string, filter SessionFilter, rows []SessionRow) ([]SessionRow, error) {
	if filter.SessionID != "" {
		return rows, nil
	}
	store, ok := h.repo.(interface {
		ListKubernetesEnvironments(context.Context) ([]*models.KubernetesEnvironment, error)
	})
	if !ok {
		return rows, nil
	}
	records, err := store.ListKubernetesEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	visibleTasks := make(map[string]bool, len(rows))
	for _, row := range rows {
		visibleTasks[row.TaskID] = true
	}
	for _, record := range records {
		if record == nil || visibleTasks[record.TaskID] || (filter.TaskID != "" && filter.TaskID != record.TaskID) || metadataString(record.Metadata, metadataResourceExecutor) != executorID {
			continue
		}
		if err := h.access.AuthorizeTaskAccess(ctx, record.TaskID); err != nil {
			if errors.Is(err, repoerrors.ErrTaskNotFound) {
				continue
			}
			return nil, err
		}
		row := retainedTaskPodRow(ctx, client, executorID, record)
		rows = append(rows, row)
	}
	return rows, nil
}

func retainedTaskPodRow(ctx context.Context, client kubeclient.Interface, executorID string, record *models.KubernetesEnvironment) SessionRow {
	run := &models.ExecutorRunning{TaskID: record.TaskID, ExecutorID: executorID, Metadata: record.Metadata}
	row := newInventorySessionRow(run)
	if !isTaskPodInventory(record.Metadata) || metadataString(record.Metadata, metadataResourceEnv) != record.EnvironmentID {
		row.FailureReason = "Kubernetes task inventory is incomplete"
		return row
	}
	if failure := validateSessionInventory(run, executorID, row); failure != "" {
		row.FailureReason = failure
		return row
	}
	pod, err := client.CoreV1().Pods(metadataString(record.Metadata, metadataNamespace)).Get(ctx, row.PodName, metav1.GetOptions{})
	if err != nil {
		row.FailureReason = podLookupFailure(err)
		return row
	}
	if !matchesSessionIdentity(pod, run) {
		row.FailureReason = "Pod identity does not match runtime inventory"
		return row
	}
	populatePodStatus(&row, pod, metadataString(record.Metadata, metadataMainContainer), models.TaskSessionStateIdle)
	return row
}
