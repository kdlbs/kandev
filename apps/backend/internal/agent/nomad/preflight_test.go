package nomad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const healthyPreflightNodes = `[{"ID":"n1","NodePool":"kandev","Status":"ready","SchedulingEligibility":"eligible","NodeResources":{"Cpu":{"CpuShares":4000},"Memory":{"MemoryMB":8192}},"ReservedResources":{"Cpu":{"CpuShares":1000},"Memory":{"MemoryMB":2048}}}]`

func preflightServer(t *testing.T, pool, scheduler string, poolStatus int) *Client {
	return preflightServerNodes(t, pool, scheduler, poolStatus, "")
}

func preflightServerNodes(t *testing.T, pool, scheduler string, poolStatus int, preflightNodes string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/node/pool/"):
			if poolStatus != 0 && poolStatus != http.StatusOK {
				http.Error(w, "missing", poolStatus)
				return
			}
			_, _ = w.Write([]byte(pool))
		case r.URL.Path == "/v1/nodes":
			nodes := preflightNodes
			if nodes == "" {
				nodes = healthyPreflightNodes
			}
			_, _ = w.Write([]byte(nodes))
		case r.URL.Path == "/v1/operator/scheduler/configuration":
			_, _ = w.Write([]byte(scheduler))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(srv.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func poolPreflightProfile(placement string, max int) ProfileConfig {
	return ProfileConfig{Mode: ModePool, NodePool: "kandev", Placement: placement, MemoryMB: 256, MemoryMaxMB: max}
}

func TestPreflightPoolAcceptsPreparedCluster(t *testing.T) {
	c := preflightServer(t, `{"Name":"kandev"}`, `{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":true}}`, 0)
	if err := c.PreflightPool(context.Background(), poolPreflightProfile(PlacementSpread, 2048)); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightPoolRefusals(t *testing.T) {
	enabled := `{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":true}}`
	disabled := `{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":false}}`
	cases := []struct {
		name    string
		client  *Client
		profile ProfileConfig
		want    string
	}{
		{"missing pool", preflightServer(t, ``, enabled, http.StatusNotFound), poolPreflightProfile(PlacementSpread, 2048), "not readable"},
		{"wrong pool", preflightServer(t, `{"Name":"other"}`, enabled, 0), poolPreflightProfile(PlacementSpread, 2048), "was not returned"},
		{"oversubscription off", preflightServer(t, `{"Name":"kandev"}`, disabled, 0), poolPreflightProfile(PlacementSpread, 2048), "oversubscription"},
		{"spread on binpack cluster", preflightServer(t, `{"Name":"kandev"}`, `{"SchedulerConfig":{"SchedulerAlgorithm":"binpack","MemoryOversubscriptionEnabled":true}}`, 0), poolPreflightProfile(PlacementSpread, 2048), "spread scheduler"},
		{"spread on binpack node pool", preflightServer(t, `{"Name":"kandev","SchedulerConfiguration":{"SchedulerAlgorithm":"binpack"}}`, enabled, 0), poolPreflightProfile(PlacementSpread, 2048), "spread scheduler"},
		{"binpack on spread cluster", preflightServer(t, `{"Name":"kandev"}`, enabled, 0), poolPreflightProfile(PlacementBinpack, 2048), "binpack scheduler"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.PreflightPool(context.Background(), tc.profile)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPreflightPoolBinpackOverrideOnNodePool(t *testing.T) {
	c := preflightServer(t, `{"Name":"kandev","SchedulerConfiguration":{"SchedulerAlgorithm":"binpack"}}`,
		`{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":true}}`, 0)
	if err := c.PreflightPool(context.Background(), poolPreflightProfile(PlacementBinpack, 2048)); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightPoolRejectsLegacyProfile(t *testing.T) {
	c := preflightServer(t, `{"Name":"kandev"}`, `{}`, 0)
	if err := c.PreflightPool(context.Background(), ProfileConfig{}); err == nil {
		t.Fatal("legacy profile must be rejected")
	}
}

func TestPreflightPoolCapacityRefusals(t *testing.T) {
	enabled := `{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":true}}`
	node := func(status, elig string, cpu, reservedCPU int) string {
		return `[{"ID":"n1","NodePool":"kandev","Status":"` + status + `","SchedulingEligibility":"` + elig + `","NodeResources":{"Cpu":{"CpuShares":` + itoa(cpu) + `},"Memory":{"MemoryMB":8192}},"ReservedResources":{"Cpu":{"CpuShares":` + itoa(reservedCPU) + `},"Memory":{"MemoryMB":2048}}}]`
	}
	cases := []struct{ name, nodes, want string }{
		{"reserved exceeds fingerprinted total", node("ready", "eligible", 48, 1000), "cpu_total_compute"},
		{"free below request", node("ready", "eligible", 1050, 1000), "no eligible node"},
		{"only ineligible node", node("ready", "ineligible", 4000, 1000), "no ready, eligible node"},
		{"only down node", node("down", "eligible", 4000, 1000), "no ready, eligible node"},
		{"no nodes", `[]`, "no ready, eligible node"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := preflightServerNodes(t, `{"Name":"kandev"}`, enabled, 0, tc.nodes)
			p := poolPreflightProfile(PlacementSpread, 2048)
			p.CPUMHz = 100
			err := c.PreflightPool(context.Background(), p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPreflightPoolCapacityAcceptsOneFitNodeAmongMisfits(t *testing.T) {
	enabled := `{"SchedulerConfig":{"SchedulerAlgorithm":"spread","MemoryOversubscriptionEnabled":true}}`
	nodes := `[{"ID":"small","NodePool":"kandev","Status":"ready","SchedulingEligibility":"eligible","NodeResources":{"Cpu":{"CpuShares":48},"Memory":{"MemoryMB":8192}},"ReservedResources":{"Cpu":{"CpuShares":1000},"Memory":{"MemoryMB":2048}}},
	{"ID":"big","NodePool":"kandev","Status":"ready","SchedulingEligibility":"eligible","NodeResources":{"Cpu":{"CpuShares":4000},"Memory":{"MemoryMB":8192}},"ReservedResources":{"Cpu":{"CpuShares":1000},"Memory":{"MemoryMB":2048}}}]`
	p := poolPreflightProfile(PlacementSpread, 2048)
	p.CPUMHz = 100
	if err := preflightServerNodes(t, `{"Name":"kandev"}`, enabled, 0, nodes).PreflightPool(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestPreflightPoolSpreadOverrideOnNodePool(t *testing.T) {
	c := preflightServer(t, `{"Name":"kandev","SchedulerConfiguration":{"SchedulerAlgorithm":"spread"}}`,
		`{"SchedulerConfig":{"SchedulerAlgorithm":"binpack","MemoryOversubscriptionEnabled":true}}`, 0)
	if err := c.PreflightPool(context.Background(), poolPreflightProfile(PlacementSpread, 2048)); err != nil {
		t.Fatalf("node pool spread override must pass: %v", err)
	}
}
