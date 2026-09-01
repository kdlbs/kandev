package github

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

//nolint:cyclop // ordered trust gates must remain visible
func (s *Service) executeCIRunDispatchFallback(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	verified *verifiedCIRun,
) (*CIRunReceipt, error) {
	if s.resolver != nil {
		dispatchClient, principal, resolveErr := s.resolveCIRunClientForPurpose(
			ctx, binding.WorkspaceID, binding.Owner, binding.Repo, CIRunOperationWorkflowDispatch,
		)
		if resolveErr != nil {
			return s.failCIRunRequest(ctx, request, ciRunFailureFromError(resolveErr))
		}
		client = dispatchClient
		setCIRunProviderPrincipal(request, principal)
	}
	if !sameRepositoryPR(verified.PR, binding) {
		return s.failCIRunRequest(ctx, request, CIRunFailureForkDispatchDisallowed)
	}
	if strings.TrimSpace(verified.PR.HeadBranch) == "" {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchRefUnavailable)
	}
	inputs, ok := reviewedWorkflowDispatchInputs(verified.Workflow.Path)
	if !ok {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	source, err := client.GetRepoFileContent(ctx, binding.Owner, binding.Repo,
		verified.Workflow.Path, verified.PR.BaseBranch)
	if err != nil {
		return s.handleCIRunWorkflowContentReadError(ctx, request, err)
	}
	if !workflowDispatchDeclared(source) || !workflowDispatchInputsAllowed(source) {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	latestBinding, latestPR, err := s.revalidateCIRunAdmission(ctx, client, request)
	if err != nil {
		return s.failCIRunAdmission(ctx, request, err)
	}
	headSource, err := client.GetRepoFileContent(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.Path, latestPR.HeadBranch)
	if err != nil {
		return s.handleCIRunWorkflowContentReadError(ctx, request, err)
	}
	if !bytes.Equal(source, headSource) {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	request.Operation = CIRunOperationWorkflowDispatch
	baseline, err := client.ListActionsWorkflowRuns(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.ID, request.ExpectedHeadSHA)
	if err != nil {
		return s.handleCIRunPreflightError(ctx, request, err)
	}
	request.ProviderRunWatermark = maxCIRunID(baseline)
	// Re-read the PR after all workflow and watermark reads. Dispatch accepts
	// only a mutable branch ref, so no earlier head check is sufficient.
	latestBinding, latestPR, err = s.revalidateCIRunAdmission(ctx, client, request)
	if err != nil || latestPR.HeadBranch != verified.PR.HeadBranch {
		if err != nil {
			return s.failCIRunAdmission(ctx, request, err)
		}
		return s.failCIRunRequest(ctx, request, CIRunFailureHeadDrift)
	}
	return s.sendCIRunDispatch(ctx, client, latestBinding, latestPR, request, verified, inputs)
}

func (s *Service) sendCIRunDispatch(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	pr *PR,
	request *CIRunRequest,
	verified *verifiedCIRun,
	inputs map[string]string,
) (*CIRunReceipt, error) {
	dispatchStartedAt := s.ciRunClock()().UTC()
	request.ProviderURL = ciRunDispatchProviderURL(binding.Owner, binding.Repo, verified.Workflow.ID)
	if err := s.auditCIRun(ctx, request, "provider_started", ""); err != nil {
		return receiptFromCIRunRequest(request), err
	}
	if err := s.store.MarkCIRunProviderCallStarted(ctx, request, dispatchStartedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.handleCIRunProviderStartConflict(ctx, client, request)
		}
		return nil, err
	}
	request.ProviderCallStartedAt = &dispatchStartedAt
	metadata, err := dispatchCIRunWorkflow(ctx, client, binding.Owner, binding.Repo,
		verified.Workflow.ID, pr.HeadBranch, inputs)
	applyCIRunProviderMetadata(request, metadata, err)
	if err != nil {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	if metadata.RunID > 0 {
		return s.reconcileReturnedCIRun(ctx, client, binding, request, verified, metadata.RunID)
	}
	return s.reconcileCIRunRequest(ctx, client, binding, request, verified)
}

func maxCIRunID(runs []GitHubActionsRun) int64 {
	var maximum int64
	for i := range runs {
		if runs[i].ID > maximum {
			maximum = runs[i].ID
		}
	}
	return maximum
}

func sameRepositoryPR(pr *PR, binding *ciRunBinding) bool {
	return pr != nil && strings.EqualFold(pr.HeadRepoOwner, binding.Owner) &&
		strings.EqualFold(pr.HeadRepoName, binding.Repo)
}

const reviewedDispatchWorkflow = ".github/workflows/e2e-tests.yml"

func reviewedWorkflowDispatchInputs(path string) (map[string]string, bool) {
	if path != reviewedDispatchWorkflow {
		return nil, false
	}
	return map[string]string{"fail_on_flaky": "false"}, true
}

func workflowDispatchDeclared(source []byte) bool {
	inOnBlock := false
	for _, line := range strings.Split(string(source), "\n") {
		withoutComment := strings.SplitN(line, "#", 2)[0]
		trimmed := strings.TrimSpace(withoutComment)
		if trimmed == "" {
			continue
		}
		indent := len(withoutComment) - len(strings.TrimLeft(withoutComment, " \t"))
		if indent == 0 {
			inOnBlock = trimmed == "on:"
			continue
		}
		if inOnBlock && (trimmed == "workflow_dispatch:" || trimmed == "workflow_dispatch: {}") {
			return true
		}
	}
	return false
}

func workflowDispatchInputsAllowed(source []byte) bool {
	dispatch, ok := workflowDispatchNode(source)
	if !ok || yamlNull(dispatch) {
		return ok
	}
	inputs, ok := yamlMappingValue(dispatch, "inputs")
	if !ok || yamlNull(inputs) {
		return true
	}
	if inputs.Kind != yaml.MappingNode {
		return false
	}
	for index := 1; index < len(inputs.Content); index += 2 {
		if !workflowDispatchInputAllowed(inputs.Content[index]) {
			return false
		}
	}
	return true
}

func workflowDispatchNode(source []byte) (*yaml.Node, bool) {
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil || len(document.Content) != 1 {
		return nil, false
	}
	on, ok := yamlMappingValue(document.Content[0], "on")
	if !ok || on.Kind != yaml.MappingNode {
		return nil, false
	}
	dispatch, ok := yamlMappingValue(on, "workflow_dispatch")
	if !ok {
		return nil, false
	}
	if !yamlNull(dispatch) && dispatch.Kind != yaml.MappingNode {
		return nil, false
	}
	return dispatch, true
}

func workflowDispatchInputAllowed(input *yaml.Node) bool {
	if input == nil || input.Kind != yaml.MappingNode {
		return false
	}
	required, present := yamlMappingValue(input, "required")
	if !present {
		return true
	}
	var isRequired bool
	if err := required.Decode(&isRequired); err != nil {
		return false
	}
	_, hasDefault := yamlMappingValue(input, "default")
	return !isRequired || hasDefault
}

func yamlNull(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

func yamlMappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}
