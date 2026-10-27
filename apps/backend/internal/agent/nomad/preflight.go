package nomad

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// nodePoolRecord is the part of a Nomad node pool preflight reads.
type nodePoolRecord struct {
	Name                   string `json:"Name"`
	SchedulerConfiguration *struct {
		SchedulerAlgorithm string `json:"SchedulerAlgorithm"`
	} `json:"SchedulerConfiguration"`
}

type schedulerConfigResponse struct {
	SchedulerConfig struct {
		SchedulerAlgorithm            string `json:"SchedulerAlgorithm"`
		MemoryOversubscriptionEnabled bool   `json:"MemoryOversubscriptionEnabled"`
	} `json:"SchedulerConfig"`
}

const (
	schedulerAlgorithmBinpack = "binpack"
	schedulerAlgorithmSpread  = "spread"
)

// PreflightPool proves the cluster can honour a pool profile before a job is
// registered: the node pool exists, memory oversubscription is on (the job asks
// for a small reservation with a larger ceiling), and the binpack placement
// mode is backed by the matching scheduler algorithm. Errors never include credentials.
func (c *Client) PreflightPool(ctx context.Context, profile ProfileConfig) error {
	if profile.Mode != ModePool {
		return errors.New("pool preflight requires a pool profile")
	}
	var pool nodePoolRecord
	if err := c.getJSON(ctx, "/v1/node/pool/"+url.PathEscape(profile.NodePool), &pool); err != nil {
		return fmt.Errorf("pool preflight: node pool %q is not readable: %w", profile.NodePool, err)
	}
	if pool.Name != profile.NodePool {
		return fmt.Errorf("pool preflight: node pool %q was not returned", profile.NodePool)
	}
	var scheduler schedulerConfigResponse
	if err := c.getJSON(ctx, "/v1/operator/scheduler/configuration", &scheduler); err != nil {
		return fmt.Errorf("pool preflight: scheduler configuration is not readable: %w", err)
	}
	if profile.MemoryMaxMB > profile.MemoryMB && !scheduler.SchedulerConfig.MemoryOversubscriptionEnabled {
		return errors.New("pool preflight: memory oversubscription is disabled in the Nomad scheduler configuration; enable it with `nomad operator scheduler set-config -memory-oversubscription=true` or set memory_max_mb equal to memory_mb")
	}
	if err := checkPlacementAlgorithm(profile, effectiveSchedulerAlgorithm(&pool, &scheduler)); err != nil {
		return err
	}
	return c.preflightPoolCapacity(ctx, profile)
}

// effectiveSchedulerAlgorithm is the node pool's scheduler algorithm when set,
// else the cluster's.
func effectiveSchedulerAlgorithm(pool *nodePoolRecord, scheduler *schedulerConfigResponse) string {
	if pool.SchedulerConfiguration != nil && pool.SchedulerConfiguration.SchedulerAlgorithm != "" {
		return pool.SchedulerConfiguration.SchedulerAlgorithm
	}
	return scheduler.SchedulerConfig.SchedulerAlgorithm
}

// checkPlacementAlgorithm refuses a placement mode the effective scheduler
// algorithm cannot honour: Nomad only spreads or packs as its algorithm says.
func checkPlacementAlgorithm(profile ProfileConfig, algorithm string) error {
	switch profile.Placement {
	case PlacementBinpack:
		if algorithm != schedulerAlgorithmBinpack {
			return fmt.Errorf("pool preflight: placement %q needs a binpack scheduler for node pool %q; set the node pool or cluster scheduler algorithm to binpack, or use placement spread", PlacementBinpack, profile.NodePool)
		}
	case PlacementSpread:
		if algorithm != schedulerAlgorithmSpread {
			return fmt.Errorf("pool preflight: placement %q needs a spread scheduler for node pool %q; set the node pool or cluster scheduler algorithm to spread (`nomad operator scheduler set-config -scheduler-algorithm=spread`), or use placement binpack", PlacementSpread, profile.NodePool)
		}
	}
	return nil
}

// preflightPoolCapacity refuses a profile no eligible pool node can ever hold.
// A node whose reserved resources reach or exceed its fingerprinted total has
// no schedulable capacity, which is how a wrong CPU fingerprint (Apple Silicon
// reports a few dozen MHz) shows up.
func (c *Client) preflightPoolCapacity(ctx context.Context, profile ProfileConfig) error {
	var nodes []NodeRecord
	path := "/v1/nodes?resources=true&node_pool=" + url.QueryEscape(profile.NodePool)
	if err := c.getJSON(ctx, path, &nodes); err != nil {
		return fmt.Errorf("pool preflight: nodes of pool %q are not readable: %w", profile.NodePool, err)
	}
	var unfit []string
	eligible := 0
	for i := range nodes {
		n := &nodes[i]
		if n.NodePool != profile.NodePool || n.Status != "ready" || n.SchedulingEligibility != "eligible" {
			continue
		}
		eligible++
		freeCPU := n.NodeResources.CPU.CPUShares - n.ReservedResources.CPU.CPUShares
		freeMem := n.NodeResources.Memory.MemoryMB - n.ReservedResources.Memory.MemoryMB
		if freeCPU >= profile.CPUMHz && freeMem >= profile.MemoryMB {
			return nil
		}
		unfit = append(unfit, fmt.Sprintf("%s (cpu %d total, %d reserved; memory %d total, %d reserved MB)", n.ID,
			n.NodeResources.CPU.CPUShares, n.ReservedResources.CPU.CPUShares, n.NodeResources.Memory.MemoryMB, n.ReservedResources.Memory.MemoryMB))
	}
	if eligible == 0 {
		return fmt.Errorf("pool preflight: node pool %q has no ready, eligible node; start a worker client or check for a quarantined node", profile.NodePool)
	}
	return fmt.Errorf("pool preflight: no eligible node in pool %q has %d MHz CPU and %d MB memory free: %s; where the client fingerprints too little CPU (Apple Silicon reports a few dozen MHz) set cpu_total_compute in the Nomad client configuration and size reserved relative to it, otherwise lower the profile's cpu_mhz or the node's reserved resources",
		profile.NodePool, profile.CPUMHz, profile.MemoryMB, strings.Join(unfit, "; "))
}
