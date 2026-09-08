package kubernetes

import (
	"context"
	"errors"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// SessionRow is a sanitized projection of one Kubernetes-backed running executor.
type SessionRow struct {
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id"`
	PodName        string `json:"pod_name,omitempty"`
	PodPhase       string `json:"pod_phase,omitempty"`
	ContainerState string `json:"container_state,omitempty"`
	Restarts       int32  `json:"restarts"`
	WorkspaceKind  string `json:"workspace_kind,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	FailureReason  string `json:"failure_reason,omitempty"`
}

// SessionImpact is the authoritative mutation impact of one Kubernetes executor.
type SessionImpact struct {
	ActiveSessionCount int `json:"active_session_count"`
}

// SessionFilter narrows an executor's authoritative running inventory before
// task authorization or Kubernetes API access.
type SessionFilter struct {
	TaskID    string
	SessionID string
}

func (h *Handler) sessionImpact(ctx context.Context, executorID string) (SessionImpact, error) {
	if executorID == "" {
		return SessionImpact{}, errExecutorIDRequired
	}
	if h.repo == nil {
		return SessionImpact{}, errSessionStatusNotWired
	}
	executor, err := h.repo.GetExecutor(ctx, executorID)
	if err != nil {
		return SessionImpact{}, err
	}
	if executor == nil {
		return SessionImpact{}, models.ErrExecutorNotFound
	}
	if executor.Type != models.ExecutorTypeKubernetes {
		return SessionImpact{}, errExecutorNotKubernetes
	}
	runs, err := h.repo.ListExecutorsRunning(ctx)
	if err != nil {
		return SessionImpact{}, err
	}
	impact := SessionImpact{}
	for _, run := range runs {
		if run != nil && run.Runtime == agentruntime.RuntimeKubernetes && run.ExecutorID == executorID &&
			isActiveExecutorRunningStatus(run.Status) {
			impact.ActiveSessionCount++
		}
	}
	return impact, nil
}

func isActiveExecutorRunningStatus(status string) bool {
	switch status {
	case models.ExecutorRunningStatusFailed,
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete:
		return false
	default:
		return true
	}
}

func (h *Handler) listSessions(
	ctx context.Context,
	executorID string,
	filter SessionFilter,
) ([]SessionRow, error) {
	if filter.SessionID != "" && filter.TaskID == "" {
		return nil, errTaskIDRequired
	}
	_, client, runs, err := h.sessionStatusSource(ctx, executorID)
	if err != nil {
		return nil, err
	}
	rows := make([]SessionRow, 0, len(runs))
	for _, run := range runs {
		if !filter.matches(run) {
			continue
		}
		row, visible, rowErr := h.sessionRow(ctx, client, executorID, run)
		if rowErr != nil {
			return nil, rowErr
		}
		if visible {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (f SessionFilter) matches(run *models.ExecutorRunning) bool {
	if run == nil {
		return f.TaskID == "" && f.SessionID == ""
	}
	if f.TaskID != "" && run.TaskID != f.TaskID {
		return false
	}
	return f.SessionID == "" || run.SessionID == f.SessionID
}

func (h *Handler) sessionStatusSource(
	ctx context.Context,
	executorID string,
) (agentkubernetes.ExecutorConfig, kubeclient.Interface, []*models.ExecutorRunning, error) {
	if executorID == "" {
		return agentkubernetes.ExecutorConfig{}, nil, nil, errExecutorIDRequired
	}
	if h.repo == nil || h.access == nil || h.clients == nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, errSessionStatusNotWired
	}
	executor, err := h.repo.GetExecutor(ctx, executorID)
	if err != nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, err
	}
	if executor == nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, models.ErrExecutorNotFound
	}
	if executor.Type != models.ExecutorTypeKubernetes {
		return agentkubernetes.ExecutorConfig{}, nil, nil, errExecutorNotKubernetes
	}
	config, err := agentkubernetes.ParseExecutorConfig(executor.Config)
	if err != nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, errSessionStatusNotWired
	}
	client, err := h.clients(config)
	if err != nil || client == nil || client.Clientset == nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, errSessionStatusNotWired
	}
	runs, err := h.repo.ListExecutorsRunning(ctx)
	if err != nil {
		return agentkubernetes.ExecutorConfig{}, nil, nil, err
	}
	return config, client.Clientset, runs, nil
}

func (h *Handler) sessionRow(
	ctx context.Context,
	client kubeclient.Interface,
	executorID string,
	run *models.ExecutorRunning,
) (SessionRow, bool, error) {
	if run == nil || run.Runtime != agentruntime.RuntimeKubernetes {
		return SessionRow{}, false, nil
	}
	if run.ExecutorID != executorID {
		return SessionRow{}, false, nil
	}
	session, err := h.repo.GetTaskSession(ctx, run.SessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return SessionRow{}, false, nil
		}
		return SessionRow{}, false, err
	}
	if session == nil || session.ID != run.SessionID || session.TaskID != run.TaskID {
		return SessionRow{}, false, nil
	}
	if err := h.access.AuthorizeTaskAccess(ctx, run.TaskID); err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return SessionRow{}, false, nil
		}
		return SessionRow{}, false, err
	}
	row := newInventorySessionRow(run)
	if inventoryFailure := validateSessionInventory(run, executorID, row); inventoryFailure != "" {
		row.FailureReason = inventoryFailure
		return row, true, nil
	}
	namespace := metadataString(run.Metadata, metadataNamespace)
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, row.PodName, metav1.GetOptions{})
	if err != nil {
		row.FailureReason = podLookupFailure(err)
		return row, true, nil
	}
	if !matchesSessionIdentity(pod, run) {
		row.FailureReason = "Pod identity does not match runtime inventory"
		return row, true, nil
	}
	populatePodStatus(&row, pod, metadataString(run.Metadata, metadataMainContainer))
	return row, true, nil
}

func newInventorySessionRow(run *models.ExecutorRunning) SessionRow {
	row := SessionRow{
		SessionID:     run.SessionID,
		TaskID:        run.TaskID,
		PodName:       metadataString(run.Metadata, metadataPodName),
		WorkspaceKind: metadataString(run.Metadata, metadataWorkspaceMode),
	}
	if !run.CreatedAt.IsZero() {
		row.CreatedAt = run.CreatedAt.UTC().Format(time.RFC3339)
	}
	return row
}

func validateSessionInventory(
	run *models.ExecutorRunning,
	executorID string,
	row SessionRow,
) string {
	identity, validIdentity := recordedResourceIdentity(run.Metadata)
	if !validIdentity || run.ID != run.SessionID || run.ExecutorID != executorID ||
		identity.ExecutorID != executorID || identity.TaskID != run.TaskID ||
		identity.SessionID != run.SessionID || metadataString(run.Metadata, metadataNamespace) == "" ||
		row.PodName == "" || metadataString(run.Metadata, metadataPodUID) == "" ||
		metadataString(run.Metadata, metadataMainContainer) == "" || row.WorkspaceKind == "" {
		return "Kubernetes runtime inventory is incomplete"
	}
	return ""
}

func matchesSessionIdentity(pod *corev1.Pod, run *models.ExecutorRunning) bool {
	if pod == nil || pod.Name != metadataString(run.Metadata, metadataPodName) ||
		pod.Namespace != metadataString(run.Metadata, metadataNamespace) ||
		string(pod.UID) != metadataString(run.Metadata, metadataPodUID) {
		return false
	}
	identity, ok := recordedResourceIdentity(run.Metadata)
	if !ok {
		return false
	}
	expected, err := agentkubernetes.OwnershipLabels(identity)
	if err != nil {
		return false
	}
	for key, value := range expected {
		if pod.Labels[key] != value {
			return false
		}
	}
	for key, value := range pod.Labels {
		if !strings.HasPrefix(key, "kandev.ai/") {
			continue
		}
		expectedValue, exists := expected[key]
		if !exists || value != expectedValue {
			return false
		}
	}
	return true
}

func recordedResourceIdentity(metadata map[string]interface{}) (agentkubernetes.ResourceIdentity, bool) {
	identity := agentkubernetes.ResourceIdentity{
		ExecutorID:    metadataString(metadata, metadataResourceExecutor),
		ProfileID:     metadataString(metadata, metadataResourceProfile),
		InstanceID:    metadataString(metadata, metadataResourceInstance),
		TaskID:        metadataString(metadata, metadataResourceTask),
		SessionID:     metadataString(metadata, metadataResourceSession),
		EnvironmentID: metadataString(metadata, metadataResourceEnv),
	}
	if _, err := agentkubernetes.OwnershipLabels(identity); err != nil {
		return agentkubernetes.ResourceIdentity{}, false
	}
	return identity, true
}

func populatePodStatus(row *SessionRow, pod *corev1.Pod, mainContainer string) {
	row.PodPhase = string(pod.Status.Phase)
	row.ContainerState = "unknown"
	failureReason := pod.Status.Reason
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != mainContainer {
			continue
		}
		row.Restarts = status.RestartCount
		switch {
		case status.State.Running != nil:
			row.ContainerState = "running"
		case status.State.Waiting != nil:
			row.ContainerState = "waiting"
			failureReason = status.State.Waiting.Reason
		case status.State.Terminated != nil:
			row.ContainerState = "terminated"
			failureReason = status.State.Terminated.Reason
		}
		break
	}
	row.FailureReason = sanitizedPodReason(failureReason)
}

func metadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func podLookupFailure(err error) string {
	switch {
	case apierrors.IsNotFound(err):
		return "Pod not found"
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return "Pod status access is denied"
	default:
		return "Pod status is unavailable"
	}
}

func sanitizedPodReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	if len(reason) > 80 {
		return "Kubernetes workload reported a failure"
	}
	for _, char := range reason {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._-", char) {
			continue
		}
		return "Kubernetes workload reported a failure"
	}
	return reason
}
