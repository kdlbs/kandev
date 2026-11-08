package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

// errStandaloneJournalOwnerBlocked reports that a retained standalone manager
// has an unresolved journal owner. Creation must stay blocked rather than
// opening the same session journal a second time.
var errStandaloneJournalOwnerBlocked = errors.New("retained standalone agent journal owner could not be reconciled")

type standaloneOwnerCandidate struct {
	info     *agentctl.InstanceInfo
	liveness reconnectOwnerLiveness
}

// reconcileRetainedStandaloneOwner resolves the exact same-session instance
// that may still own the session journal before a replacement opens it. A live
// owner is adopted, stopped owners are disposed only after every relevant
// candidate is verified, and unknown evidence blocks creation.
func (r *StandaloneExecutor) reconcileRetainedStandaloneOwner(
	ctx context.Context,
	control *agentctl.ControlClient,
	lease *agentctl.RuntimeLease,
	req *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
	if control == nil {
		return nil, standaloneOwnerBlocked(errors.New("agentctl instance inventory is unavailable"))
	}
	instances, err := control.ListInstances(ctx)
	if err != nil {
		return nil, standaloneOwnerBlocked(fmt.Errorf("list agent instances before journal open: %w", err))
	}
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(instances))
	candidates, err := r.classifyStandaloneJournalOwners(ctx, lease, req, instances, seen)
	if err != nil {
		return nil, err
	}
	if err := verifyStandaloneOwnerAbsence(ctx, control, lease, req, seen); err != nil {
		return nil, err
	}
	live, stopped, err := splitStandaloneOwnerCandidates(candidates)
	if err != nil {
		return nil, err
	}
	if len(live) > 1 {
		return nil, standaloneOwnerBlocked(fmt.Errorf("multiple live agent instances (%d) claim the same session journal", len(live)))
	}
	if len(live) == 1 {
		return r.buildAdoptedStandaloneInstance(lease, req, live[0].info)
	}
	for _, candidate := range stopped {
		if err := r.disposeStoppedStandaloneOwner(ctx, control, lease, candidate.info.ID); err != nil {
			return nil, err
		}
	}
	if len(stopped) > 0 {
		if err := recheckNoStandaloneOwners(ctx, control, req); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (r *StandaloneExecutor) classifyStandaloneJournalOwners(
	ctx context.Context,
	lease *agentctl.RuntimeLease,
	req *ExecutorCreateRequest,
	instances []*agentctl.InstanceInfo,
	seen map[string]struct{},
) ([]standaloneOwnerCandidate, error) {
	candidates := make([]standaloneOwnerCandidate, 0, len(instances))
	for _, info := range instances {
		if info == nil || info.ID == "" {
			return nil, standaloneOwnerBlocked(errors.New("agentctl returned an incomplete instance inventory"))
		}
		if _, duplicate := seen[info.ID]; duplicate {
			return nil, standaloneOwnerBlocked(fmt.Errorf("agent instance %q was reported more than once", info.ID))
		}
		seen[info.ID] = struct{}{}
		relevant, relevanceErr := standaloneOwnerRelevant(req, info)
		if relevanceErr != nil {
			return nil, standaloneOwnerBlocked(relevanceErr)
		}
		if !relevant {
			continue
		}
		if len(candidates) >= maxReconnectOwnerCandidates {
			return nil, standaloneOwnerBlocked(fmt.Errorf("more than %d agent instances claim session %q", maxReconnectOwnerCandidates, req.SessionID))
		}
		if err := verifyStandaloneOwnerIdentity(req, info); err != nil {
			return nil, standaloneOwnerBlocked(err)
		}
		liveness, err := r.standaloneOwnerLiveness(ctx, lease, req, info)
		if err != nil {
			return nil, standaloneOwnerBlocked(err)
		}
		candidates = append(candidates, standaloneOwnerCandidate{info: info, liveness: liveness})
	}
	return candidates, nil
}

// standaloneOwnerRelevant reports whether info can reference the requested
// execution, its recorded predecessor, or another owner of the same session.
// A different session keeps its own independent journal.
func standaloneOwnerRelevant(req *ExecutorCreateRequest, info *agentctl.InstanceInfo) (bool, error) {
	requested := info.ID == req.InstanceID && req.InstanceID != ""
	predecessor := req.PreviousExecutionID != "" && info.ID == req.PreviousExecutionID
	matchesRecorded := requested || predecessor
	sameSession := req.SessionID != "" && info.SessionID == req.SessionID
	if info.SessionID != "" && info.SessionID != req.SessionID {
		if matchesRecorded {
			return false, fmt.Errorf("agent instance %q belongs to a different session", info.ID)
		}
		return false, nil
	}
	if info.TaskID != "" && req.TaskID != "" && info.TaskID != req.TaskID {
		if matchesRecorded || sameSession {
			return false, fmt.Errorf("agent instance %q belongs to a different task", info.ID)
		}
		return false, nil
	}
	return matchesRecorded || sameSession, nil
}

func verifyStandaloneOwnerIdentity(req *ExecutorCreateRequest, info *agentctl.InstanceInfo) error {
	if info.Port <= 0 {
		return fmt.Errorf("agent instance %q has no reachable port", info.ID)
	}
	if info.WorkspacePath != "" && req.WorkspacePath != "" &&
		filepath.Clean(info.WorkspacePath) != filepath.Clean(req.WorkspacePath) {
		return fmt.Errorf("agent instance %q owns a different workspace", info.ID)
	}
	return nil
}

func (r *StandaloneExecutor) standaloneOwnerLiveness(
	ctx context.Context,
	lease *agentctl.RuntimeLease,
	req *ExecutorCreateRequest,
	info *agentctl.InstanceInfo,
) (reconnectOwnerLiveness, error) {
	client := r.standaloneInstanceClient(lease, info.Port, info.ID, req.SessionID)
	if client == nil {
		return reconnectOwnerUnknown, fmt.Errorf("agent instance %q has no authenticated client", info.ID)
	}
	defer client.Close()
	status, err := client.GetStatus(ctx)
	if err != nil {
		return reconnectOwnerUnknown, fmt.Errorf("read agent instance %q status: %w", info.ID, err)
	}
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return reconnectOwnerUnknown, err
	}
	if status == nil {
		return reconnectOwnerUnknown, fmt.Errorf("agent instance %q status is missing", info.ID)
	}
	if !status.IsAgentRunning() {
		return reconnectOwnerStopped, nil
	}
	if err := validateStandaloneOwnerDelivery(ctx, client, req); err != nil {
		return reconnectOwnerUnknown, err
	}
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return reconnectOwnerUnknown, err
	}
	return reconnectOwnerLive, nil
}

func validateStandaloneOwnerDelivery(ctx context.Context, client *agentctl.Client, req *ExecutorCreateRequest) error {
	descriptor, err := client.GetDeliveryStatus(ctx, req.DeliveryStreamID)
	if err != nil {
		if isUnsupportedStandaloneDeliveryRoute(err) {
			// A positively identified legacy peer keeps its existing eligible
			// adoption behavior. Missing diagnostics never authorize takeover.
			return nil
		}
		return fmt.Errorf("read delivery identity: %w", err)
	}
	return validateStandaloneDeliveryDescriptor(req, descriptor)
}

func isUnsupportedStandaloneDeliveryRoute(err error) bool {
	var httpErr *agentctl.DeliveryHTTPError
	return errors.As(err, &httpErr) &&
		(httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusMethodNotAllowed)
}

func validateStandaloneDeliveryDescriptor(req *ExecutorCreateRequest, descriptor *agentctl.DeliveryStatus) error {
	if descriptor == nil {
		return nil
	}
	switch {
	case descriptor.SessionID != "" && descriptor.SessionID != req.SessionID:
		return errors.New("delivery identity conflicts with the requested session")
	case req.DeliveryStreamID != "" && descriptor.StreamID != "" && descriptor.StreamID != req.DeliveryStreamID:
		return errors.New("delivery stream conflicts with the requested stream")
	case req.DeliveryIncarnationID != "" && descriptor.IncarnationID != "" && descriptor.IncarnationID != req.DeliveryIncarnationID:
		return errors.New("delivery incarnation conflicts with the requested incarnation")
	case req.DeliveryHarnessGeneration > 0 && descriptor.HarnessGeneration > 0 &&
		descriptor.HarnessGeneration != req.DeliveryHarnessGeneration:
		return errors.New("delivery harness generation conflicts with the requested generation")
	}
	return nil
}

func (r *StandaloneExecutor) buildAdoptedStandaloneInstance(
	lease *agentctl.RuntimeLease,
	req *ExecutorCreateRequest,
	info *agentctl.InstanceInfo,
) (*ExecutorInstance, error) {
	client := r.standaloneInstanceClient(lease, info.Port, info.ID, req.SessionID)
	if client == nil {
		return nil, standaloneOwnerBlocked(fmt.Errorf("agent instance %q has no authenticated client", info.ID))
	}
	metadata := buildStandaloneInstanceMetadata(req, info.Port)
	metadata[MetadataKeyReuseExistingProcess] = true
	if req.RetainedBuiltinBinding != nil {
		// The persisted binding is the retained helper's authority. Restore it
		// unchanged so later verification compares against the same profile.
		metadata[mcpprofile.BuiltinBindingMetadataKey] = req.RetainedBuiltinBinding
	} else if req.McpProfile != nil && req.McpProfile.BuiltinPackages != nil {
		client.Close()
		return nil, standaloneOwnerBlocked(errors.New("retained agent has no persisted built-in MCP binding for the requested workspace"))
	}
	if info.WorkspaceSourceRoots == nil && len(req.WorkspaceSourceRoots) > 0 {
		client.Close()
		return nil, standaloneOwnerBlocked(errors.New("retained agent workspace source roots are unavailable"))
	}
	workspacePath := info.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.WorkspacePath
	}
	return &ExecutorInstance{
		InstanceID:                 info.ID,
		TaskID:                     req.TaskID,
		SessionID:                  req.SessionID,
		RuntimeName:                r.Name(),
		Client:                     client,
		StandaloneInstanceID:       info.ID,
		StandalonePort:             info.Port,
		WorkspacePath:              workspacePath,
		Metadata:                   metadata,
		ReusedWorkspace:            true,
		Env:                        info.Env,
		WorkspaceSourceRoots:       append([]string{}, info.WorkspaceSourceRoots...),
		ProviderSessionID:          info.ProviderSessionID,
		CopilotNativeSessionEvents: info.CopilotNativeSessionEvents,
	}, nil
}

func (r *StandaloneExecutor) standaloneInstanceClient(
	lease *agentctl.RuntimeLease,
	port int,
	instanceID, sessionID string,
) *agentctl.Client {
	if lease != nil {
		return lease.NewBoundInstanceClient(port, r.logger,
			agentctl.WithExecutionID(instanceID), agentctl.WithSessionID(sessionID))
	}
	if r.ctl == nil {
		return nil
	}
	return agentctl.NewClient(r.host, port, r.logger,
		agentctl.WithExecutionID(instanceID),
		agentctl.WithSessionID(sessionID),
		agentctl.WithAuthToken(r.authToken))
}

// disposeStoppedStandaloneOwner releases the retained manager's journal handle
// through the exact DELETE contract, then confirms authenticated absence. A
// lost response is reconciled through that same binding; an unconfirmed result
// blocks the later journal open.
func (r *StandaloneExecutor) disposeStoppedStandaloneOwner(
	ctx context.Context,
	control *agentctl.ControlClient,
	lease *agentctl.RuntimeLease,
	instanceID string,
) error {
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return err
	}
	deleteErr := control.DeleteInstance(ctx, instanceID)
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return err
	}
	info, verifyErr := control.GetInstance(ctx, instanceID)
	if err := checkStandaloneOwnerLease(lease); err != nil {
		return err
	}
	switch {
	case verifyErr == nil && info != nil:
		if deleteErr != nil {
			return standaloneOwnerBlocked(fmt.Errorf("dispose stopped agent instance %q before journal reopen: %w", instanceID, deleteErr))
		}
		return standaloneOwnerBlocked(fmt.Errorf("agent instance %q still owns its journal after disposal", instanceID))
	case verifyErr != nil && !errors.Is(verifyErr, agentctl.ErrInstanceNotFound):
		return standaloneOwnerBlocked(fmt.Errorf("reconcile disposal of agent instance %q: %w", instanceID, errors.Join(deleteErr, verifyErr)))
	}
	if deleteErr != nil {
		r.logger.Warn("disposed stopped standalone journal owner after a lost DELETE response",
			zap.String("instance_id", instanceID), zap.Error(deleteErr))
	}
	return nil
}

func verifyStandaloneOwnerAbsence(
	ctx context.Context,
	control *agentctl.ControlClient,
	lease *agentctl.RuntimeLease,
	req *ExecutorCreateRequest,
	seen map[string]struct{},
) error {
	for _, id := range []string{req.InstanceID, req.PreviousExecutionID} {
		if id == "" {
			continue
		}
		if _, listed := seen[id]; listed {
			continue
		}
		info, err := control.GetInstance(ctx, id)
		if leaseErr := checkStandaloneOwnerLease(lease); leaseErr != nil {
			return leaseErr
		}
		if err != nil {
			if errors.Is(err, agentctl.ErrInstanceNotFound) {
				continue
			}
			return standaloneOwnerBlocked(fmt.Errorf("verify absence of agent instance %q: %w", id, err))
		}
		if info != nil {
			return standaloneOwnerBlocked(fmt.Errorf("agent instance %q was not returned by the instance inventory", id))
		}
	}
	return nil
}

func recheckNoStandaloneOwners(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
) error {
	instances, err := control.ListInstances(ctx)
	if err != nil {
		return standaloneOwnerBlocked(fmt.Errorf("recheck agent instance inventory after disposal: %w", err))
	}
	for _, info := range instances {
		if info == nil || info.ID == "" {
			return standaloneOwnerBlocked(errors.New("agentctl returned an incomplete instance inventory after disposal"))
		}
		relevant, relevanceErr := standaloneOwnerRelevant(req, info)
		if relevanceErr != nil {
			return standaloneOwnerBlocked(relevanceErr)
		}
		if relevant {
			return standaloneOwnerBlocked(fmt.Errorf("agent instance %q still claims session %q after disposal", info.ID, req.SessionID))
		}
	}
	return nil
}

func splitStandaloneOwnerCandidates(candidates []standaloneOwnerCandidate) (live, stopped []standaloneOwnerCandidate, err error) {
	live = make([]standaloneOwnerCandidate, 0, 1)
	stopped = make([]standaloneOwnerCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.liveness {
		case reconnectOwnerLive:
			live = append(live, candidate)
		case reconnectOwnerStopped:
			stopped = append(stopped, candidate)
		default:
			return nil, nil, standaloneOwnerBlocked(fmt.Errorf("agent instance %q ownership is unresolved", candidate.info.ID))
		}
	}
	return live, stopped, nil
}

func checkStandaloneOwnerLease(lease *agentctl.RuntimeLease) error {
	if lease == nil {
		return nil
	}
	if err := lease.CheckCurrent(); err != nil {
		return standaloneOwnerBlocked(fmt.Errorf("standalone runtime retired while reconciling the session journal: %w", err))
	}
	return nil
}

func standaloneOwnerBlocked(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errStandaloneJournalOwnerBlocked) {
		return err
	}
	return fmt.Errorf("%w: %w", errStandaloneJournalOwnerBlocked, err)
}
