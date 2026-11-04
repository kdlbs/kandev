package lifecycle

import (
	"context"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// errReconnectOwnerBlocked reports that a retained container has an
// unresolved journal owner. Recovery must stay blocked rather than falling
// back to a fresh launch that would abandon the retained workspace.
var errReconnectOwnerBlocked = errors.New("previous agent journal owner could not be reconciled")

// maxReconnectOwnerCandidates bounds one reconnect inventory read. A
// container with more same-session instances than this is ambiguous and must
// not be mutated automatically.
const maxReconnectOwnerCandidates = 8

// reconnectInventory is the bounded authenticated instance inventory used to
// find a previous journal owner. It is separate from the narrower creation
// interface so an older peer reports a blocked outcome instead of guessing.
type reconnectInventory interface {
	ListInstances(context.Context) ([]*agentctl.InstanceInfo, error)
}

type reconnectOwnerLiveness int

const (
	reconnectOwnerUnknown reconnectOwnerLiveness = iota
	reconnectOwnerStopped
	reconnectOwnerLive
)

type reconnectOwnerCandidate struct {
	info     *agentctl.InstanceInfo
	liveness reconnectOwnerLiveness
}

// reconcilePreviousJournalOwner resolves the exact same-session instance that
// may still own the owner-scoped delivery journal before a replacement opens
// it. It adopts a live owner and disposes stopped owners. Any unknown or
// ambiguous evidence returns an error so recovery stays blocked and the
// workspace and journal remain intact.
func (r *DockerExecutor) reconcilePreviousJournalOwner(
	ctx context.Context,
	dockerClient hostPortLookup,
	ctl reconnectControlClient,
	req *ExecutorCreateRequest,
	containerID, containerIP, requestedID, previousID, authToken string,
) (port int, reusingProcess bool, instanceID string, handled bool, err error) {
	inventory, ok := ctl.(reconnectInventory)
	if !ok {
		return 0, false, "", false, errors.New("agentctl instance inventory is unavailable; refusing to create a replacement journal owner")
	}
	instances, err := inventory.ListInstances(ctx)
	if err != nil {
		return 0, false, "", false, fmt.Errorf("list agentctl instances before journal reopen: %w", err)
	}
	candidates, err := r.classifyReconnectOwners(
		ctx, dockerClient, req, containerID, containerIP, requestedID, previousID, authToken, instances,
	)
	if err != nil {
		return 0, false, "", false, err
	}
	if len(candidates) == 0 {
		return 0, false, "", false, nil
	}

	live := make([]reconnectOwnerCandidate, 0, 1)
	stopped := make([]reconnectOwnerCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.liveness == reconnectOwnerLive {
			live = append(live, candidate)
		} else {
			stopped = append(stopped, candidate)
		}
	}
	if len(live) > 1 {
		return 0, false, "", false, fmt.Errorf("multiple live agent instances (%d) claim the same session journal", len(live))
	}
	for _, candidate := range stopped {
		if disposeErr := ctl.DeleteInstance(ctx, candidate.info.ID); disposeErr != nil {
			return 0, false, "", false, fmt.Errorf("dispose stopped agent instance %q before journal reopen: %w", candidate.info.ID, disposeErr)
		}
	}
	if len(live) == 1 {
		return live[0].info.Port, true, live[0].info.ID, true, nil
	}
	return 0, false, "", false, nil
}

func (r *DockerExecutor) classifyReconnectOwners(
	ctx context.Context,
	dockerClient hostPortLookup,
	req *ExecutorCreateRequest,
	containerID, containerIP, requestedID, previousID, authToken string,
	instances []*agentctl.InstanceInfo,
) ([]reconnectOwnerCandidate, error) {
	candidates := make([]reconnectOwnerCandidate, 0, len(instances))
	seen := make(map[string]struct{}, len(instances))
	for _, info := range instances {
		if info == nil || info.ID == "" || info.ID == requestedID {
			continue
		}
		if _, duplicate := seen[info.ID]; duplicate {
			return nil, fmt.Errorf("agent instance %q was reported more than once", info.ID)
		}
		seen[info.ID] = struct{}{}
		relevant, conflictErr := reconnectOwnerRelevant(req, info, previousID)
		if conflictErr != nil {
			return nil, conflictErr
		}
		if !relevant {
			continue
		}
		if len(candidates) >= maxReconnectOwnerCandidates {
			return nil, fmt.Errorf("more than %d agent instances claim session %q", maxReconnectOwnerCandidates, req.SessionID)
		}
		liveness, livenessErr := r.reconnectOwnerLiveness(ctx, dockerClient, req, containerID, containerIP, info, authToken)
		if livenessErr != nil {
			return nil, fmt.Errorf("classify agent instance %q before journal reopen: %w", info.ID, livenessErr)
		}
		candidates = append(candidates, reconnectOwnerCandidate{info: info, liveness: liveness})
	}
	return candidates, nil
}

func reconnectOwnerRelevant(req *ExecutorCreateRequest, info *agentctl.InstanceInfo, previousID string) (bool, error) {
	if info.SessionID != "" && info.SessionID != req.SessionID {
		// A different session legitimately owns its own instance and journal
		// inside the retained container.
		return false, nil
	}
	if info.TaskID != "" && req.TaskID != "" && info.TaskID != req.TaskID {
		return false, fmt.Errorf("agent instance %q belongs to a different task", info.ID)
	}
	if info.ID == previousID {
		return true, nil
	}
	if req.SessionID == "" || info.SessionID != req.SessionID {
		return false, nil
	}
	return true, nil
}

func (r *DockerExecutor) reconnectOwnerLiveness(
	ctx context.Context,
	dockerClient hostPortLookup,
	req *ExecutorCreateRequest,
	containerID, containerIP string,
	info *agentctl.InstanceInfo,
	authToken string,
) (reconnectOwnerLiveness, error) {
	if info.Port <= 0 {
		return reconnectOwnerUnknown, errors.New("agent instance has no reachable port")
	}
	host, port, err := r.resolveEndpoint(ctx, dockerClient, containerID, info.Port, containerIP)
	if err != nil {
		return reconnectOwnerUnknown, err
	}
	client := agentctl.NewClient(host, port, r.logger,
		agentctl.WithExecutionID(info.ID), agentctl.WithAuthToken(authToken))
	defer client.Close()
	status, err := client.GetStatus(ctx)
	if err != nil {
		return reconnectOwnerUnknown, fmt.Errorf("read agent process status: %w", err)
	}
	if status == nil {
		return reconnectOwnerUnknown, errors.New("agent process status is missing")
	}
	if !status.IsAgentRunning() {
		return reconnectOwnerStopped, nil
	}
	if err := validateReconnectDeliveryIdentity(ctx, client, req); err != nil {
		return reconnectOwnerUnknown, err
	}
	return reconnectOwnerLive, nil
}

func validateReconnectDeliveryIdentity(
	ctx context.Context,
	client *agentctl.Client,
	req *ExecutorCreateRequest,
) error {
	descriptor, err := client.GetDeliveryStatus(ctx, req.DeliveryStreamID)
	if err != nil {
		var httpErr *agentctl.DeliveryHTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == 404 || httpErr.StatusCode == 405) {
			// A positively identified legacy peer keeps its existing eligible
			// adoption behavior. Missing diagnostics never authorize a
			// replacement or a takeover.
			return nil
		}
		return fmt.Errorf("read delivery identity: %w", err)
	}
	if descriptor == nil {
		return nil
	}
	if descriptor.SessionID != "" && descriptor.SessionID != req.SessionID {
		return errors.New("delivery identity conflicts with the requested session")
	}
	return nil
}
