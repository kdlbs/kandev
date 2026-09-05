package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// KubernetesPreparer emits launch progress; cluster/resource validation stays
// in KubernetesExecutor so it uses the same client and identity checks as launch.
type KubernetesPreparer struct {
	logger *logger.Logger
}

func NewKubernetesPreparer(log *logger.Logger) *KubernetesPreparer {
	return &KubernetesPreparer{logger: log.WithFields(zap.String("component", "kubernetes-preparer"))}
}

func (p *KubernetesPreparer) Name() string { return "k8s" }

func (p *KubernetesPreparer) Prepare(
	_ context.Context,
	req *EnvPrepareRequest,
	onProgress PrepareProgressCallback,
) (*EnvPrepareResult, error) {
	started := time.Now()
	step := beginStep("Validate Kubernetes executor configuration")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	reportProgress(onProgress, step, 1, 1)
	result := &EnvPrepareResult{Success: true, Steps: []PrepareStep{step}, Duration: time.Since(started)}
	if req != nil {
		result.WorkspacePath = req.WorkspacePath
		result.WorktreeBranch = nonWorktreeTaskBranch(req)
	}
	p.logger.Debug("prepared kubernetes environment")
	return result, nil
}
