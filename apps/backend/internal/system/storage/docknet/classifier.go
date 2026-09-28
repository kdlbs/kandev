// Package docknet reclaims Docker bridge networks whose owning task work is
// finished, restoring the daemon's address pool without ever touching a
// network a live task or container still needs (ADR 0009 fail-closed
// semantics: ambiguity keeps the network and sheds the decision to the next
// cycle).
package docknet

import (
	"context"
	"strings"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
)

// Classification labels one network's removal eligibility. The stable IDs
// ship in run results and metrics.
type Classification string

const (
	// ClassActive is a network with at least one connected container and an
	// owning environment that resolves to an active task. Never a candidate.
	ClassActive Classification = "active"
	// ClassAttached carries kandev ownership labels whose owning task row
	// exists and is neither archived nor terminal, with zero connected
	// containers. Never auto-removed.
	ClassAttached Classification = "attached"
	// ClassOrphaned has zero connected containers and an owning task that is
	// archived or terminal and past its grace window. Eligible for quarantined
	// removal only.
	ClassOrphaned Classification = "orphaned"
	// ClassSafelyStale has zero connected containers, no resolvable owner, and
	// a persisted first-seen observation at least the stable-age threshold old.
	// Eligible for quarantined removal only.
	ClassSafelyStale Classification = "safely_stale"
	// ClassStaleUncertain is any network whose classification depends on a
	// failed read (task store, inspect) or whose grace/stable thresholds have
	// not elapsed. Fail-closed: kept, retried next cycle.
	ClassStaleUncertain Classification = "stale_uncertain"
	// ClassExcluded is a network the provider never considers: preinstalled
	// special networks, non-bridge drivers, and config-only placeholders.
	ClassExcluded Classification = "excluded"
)

// TaskLookup is the answer the task oracle gives for one ownership key.
type TaskLookup int

const (
	// TaskLookupActive means the owning task row exists, is not archived, and
	// is not terminal.
	TaskLookupActive TaskLookup = iota
	// TaskLookupInactive means the owning task row exists and is archived or
	// terminal.
	TaskLookupInactive
	// TaskLookupUnknown means no owning task row matches the key.
	TaskLookupUnknown
)

// TaskOracle resolves an ownership key (kandev task ID label or compose
// project name) to the owning task's liveness. An error means classification
// must fail closed for the network.
type TaskOracle interface {
	// Ownership returns the resolved ownership key for the network's labels
	// and the lookup result for that key.
	Ownership(ctx context.Context, network agentdocker.NetworkInfo) (key string, lookup TaskLookup, err error)
}

// Evidence records why a network received its classification; it is persisted
// with the run result so a dry run shows exactly what would be removed and
// why every kept network was kept.
type Evidence struct {
	ConnectedContainers int    `json:"connected_containers"`
	OwnershipKey        string `json:"ownership_key"`
	FirstSeenAt         string `json:"first_seen_at,omitempty"`
	FirstSeenStable     bool   `json:"first_seen_stable"`
	PastGraceWindow     bool   `json:"past_grace_window"`
	Reason              string `json:"reason"`
}

// ClassifiedNetwork pairs a census network with its classification evidence.
type ClassifiedNetwork struct {
	Network  agentdocker.NetworkInfo `json:"network"`
	Class    Classification          `json:"class"`
	Evidence Evidence                `json:"evidence"`
}

// ClassifyOptions carries the thresholds and clock used by Classify.
type ClassifyOptions struct {
	// Now is the clock used for grace-window arithmetic; tests inject fixed times.
	Now func() time.Time
}

// preinstalledNetworkNames are the daemon's built-in networks. "bridge" here
// is the named default bridge, not the bridge driver: task compose projects
// also use the bridge driver and must stay in scope.
var preinstalledNetworkNames = map[string]struct{}{
	"bridge": {},
	"host":   {},
	"none":   {},
}

// IsExcluded reports whether a network is outside the provider's scope
// entirely: preinstalled special networks, non-bridge drivers, swarm scope,
// and config-only placeholders.
func IsExcluded(network agentdocker.NetworkInfo) bool {
	if _, special := preinstalledNetworkNames[network.Name]; special {
		return true
	}
	if network.Driver != "bridge" {
		return true
	}
	return network.Scope == "swarm" || network.ConfigOnly
}

// Classify applies the fail-closed decision tree to one census network.
func Classify(
	ctx context.Context,
	network agentdocker.NetworkInfo,
	oracle TaskOracle,
	firstSeen time.Time,
	graceWindow, staleAge time.Duration,
	options ClassifyOptions,
) ClassifiedNetwork {
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	evidence := Evidence{}
	if !firstSeen.IsZero() {
		evidence.FirstSeenAt = firstSeen.UTC().Format(time.RFC3339)
	}
	reference := now().UTC()
	if !firstSeen.IsZero() {
		evidence.FirstSeenStable = reference.Sub(firstSeen) >= staleAge
		evidence.PastGraceWindow = reference.Sub(firstSeen) >= graceWindow
	}

	if IsExcluded(network) {
		return ClassifiedNetwork{
			Network: network, Class: ClassExcluded,
			Evidence: evidenceWith(evidence, "preinstalled or out-of-scope network"),
		}
	}
	key, lookup, err := oracle.Ownership(ctx, network)
	evidence.OwnershipKey = key
	if err != nil {
		return ClassifiedNetwork{
			Network: network, Class: ClassStaleUncertain,
			Evidence: evidenceWith(evidence, "task oracle read failed; kept pending next cycle"),
		}
	}
	if len(network.Containers) > 0 {
		// A connected container makes this active regardless of the oracle's
		// answer: fail-closed means an attached network is never a candidate.
		evidence.ConnectedContainers = len(network.Containers)
		return ClassifiedNetwork{
			Network: network, Class: ClassActive,
			Evidence: evidenceWith(evidence, "network has connected containers"),
		}
	}
	switch lookup {
	case TaskLookupActive:
		return ClassifiedNetwork{
			Network: network, Class: ClassAttached,
			Evidence: evidenceWith(evidence, "owning task is active"),
		}
	case TaskLookupInactive:
		if !evidence.PastGraceWindow {
			return ClassifiedNetwork{
				Network: network, Class: ClassStaleUncertain,
				Evidence: evidenceWith(evidence, "owning task finished but grace window still running"),
			}
		}
		return ClassifiedNetwork{
			Network: network, Class: ClassOrphaned,
			Evidence: evidenceWith(evidence, "owning task finished and grace window elapsed"),
		}
	default:
		if evidence.FirstSeenStable {
			return ClassifiedNetwork{
				Network: network, Class: ClassSafelyStale,
				Evidence: evidenceWith(evidence, "no owner and first-seen age past stable threshold"),
			}
		}
		return ClassifiedNetwork{
			Network: network, Class: ClassStaleUncertain,
			Evidence: evidenceWith(evidence, "no resolvable owner yet; observing for stable age"),
		}
	}
}

// Eligible reports whether a classified network may enter the quarantine
// ledger's removal path this cycle.
func Eligible(class Classification) bool {
	return class == ClassOrphaned || class == ClassSafelyStale
}

func evidenceWith(evidence Evidence, reason string) Evidence {
	evidence.Reason = reason
	return evidence
}

// OwnershipKeyFromLabels resolves the strongest ownership signal present on a
// network's labels. kandev.* labels win over compose labels; compose project
// labels are the only signal on broker-launched projects today. A label-less
// network returns "".
func OwnershipKeyFromLabels(labels map[string]string) string {
	if taskID := strings.TrimSpace(labels["kandev.task_id"]); taskID != "" {
		return taskID
	}
	if project := strings.TrimSpace(labels["com.docker.compose.project"]); project != "" {
		return project
	}
	return ""
}
