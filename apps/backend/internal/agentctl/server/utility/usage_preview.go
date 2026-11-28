package utility

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
	"net/http"
)

// UsagePreviewRequest contains the same prepared config as actual inference,
// without a prompt, attempt, provider call or newly allocated execution owner.
type UsagePreviewRequest struct {
	ExecutionID     string                     `json:"execution_id"`
	AgentID         string                     `json:"agent_id"`
	Model           string                     `json:"model"`
	Policy          usage.ExecutionUsagePolicy `json:"policy"`
	InferenceConfig *InferenceConfigDTO        `json:"inference_config"`
}

func (h *Handler) handleUsagePreview(c *gin.Context) {
	var request UsagePreviewRequest
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage preview"})
		return
	}
	reader, ok := h.acpExecutor.(interface {
		PreviewExecutionUsage(context.Context, *UsagePreviewRequest) *usage.ExecutionUsageReport
	})
	if !ok {
		c.JSON(http.StatusOK, &usage.ExecutionUsageReport{Reason: "execution_unknown"})
		return
	}
	c.JSON(http.StatusOK, reader.PreviewExecutionUsage(c.Request.Context(), &request))
}

func (e *ACPInferenceExecutor) PreviewExecutionUsage(ctx context.Context, request *UsagePreviewRequest) *usage.ExecutionUsageReport {
	unknown := &usage.ExecutionUsageReport{Reason: "execution_unknown"}
	if request == nil || request.InferenceConfig == nil || request.ExecutionID == "" {
		return unknown
	}
	cfg := request.InferenceConfig
	if cfg.WorkDir == "" || len(cfg.Command) == 0 || cfg.ProviderGatewayAuth != nil || (cfg.Protocol != "" && cfg.Protocol != agent.ProtocolACP) {
		return unknown
	}
	req := &PromptRequest{AgentID: request.AgentID, Model: request.Model, InferenceConfig: cfg, UsagePolicy: &request.Policy, AttemptID: request.ExecutionID}
	input := nativeInferenceUsageInput(req, sanitizeEnvForAgent(cfg))
	return e.executionUsage.PeekPreparedExecutionUsage(ctx, input)
}

func nativeInferenceUsageInput(req *PromptRequest, environment []string) usage.ExecutionUsageInput {
	policy := *req.UsagePolicy
	policy.AgentID = req.AgentID
	executionID := req.AttemptID
	if executionID == "" {
		executionID = req.CallID
	}
	input := usage.ExecutionInputFromEnvironment(policy, executionID, req.Model, environment)
	input.WorkDir = req.InferenceConfig.WorkDir
	// A wrapper can alter the child environment after this snapshot. Do not
	// claim that the backend's visible credentials identify its account.
	if !req.InferenceConfig.LaunchWrapper.IsZero() {
		return input
	}
	input.Command = append(append(append([]string(nil), req.InferenceConfig.CommandPrefix...), buildACPCommand(req.InferenceConfig, req.Model)...), req.InferenceConfig.CLIFlags...)
	return input
}
