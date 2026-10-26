package nomad

import (
	"strings"
	"testing"
	"time"
)

func poolTestProfile() ProfileConfig {
	return ProfileConfig{
		Mode: ModePool, Namespace: "default", Region: "global", Datacenter: "dc1", Platform: WorkerPlatform,
		NodePool: "kandev", Placement: PlacementSpread, CPUMHz: 100, MemoryMB: 256, MemoryMaxMB: 2048,
		DisconnectLostAfter: time.Hour,
	}
}

func poolTestState(t *testing.T) EnvironmentState {
	t.Helper()
	s := poolState("intent")
	s.DisconnectLostAfterSeconds = 3600
	id, err := StableJobID(JobIdentity{EnvironmentID: s.EnvironmentID, Generation: s.Generation, OperationID: s.OperationID, InstallationID: s.InstallationID})
	if err != nil {
		t.Fatal(err)
	}
	s.JobID = id
	return s
}

func buildPool(t *testing.T, p ProfileConfig) Job {
	t.Helper()
	job, err := BuildPoolJob(PoolJobInput{Profile: p, State: poolTestState(t), ArtifactDigest: strings.Repeat("c", 64)})
	if err != nil {
		t.Fatalf("BuildPoolJob: %v", err)
	}
	return job
}

func hasConstraint(job Job, ltarget, operand, rtarget string) bool {
	for _, c := range job.Constraints {
		if c.LTarget == ltarget && c.Operand == operand && c.RTarget == rtarget {
			return true
		}
	}
	return false
}

func TestPoolJobNamesNoNodeAndRequiresPreparedNodes(t *testing.T) {
	job := buildPool(t, poolTestProfile())
	if job.NodePool != "kandev" {
		t.Fatalf("node pool = %q", job.NodePool)
	}
	for _, c := range job.Constraints {
		if c.LTarget == "${node.unique.id}" {
			t.Fatalf("unpinned pool job names a node: %+v", c)
		}
	}
	for _, key := range []string{MetaAttach, MetaStateRoot} {
		if !hasConstraint(job, "${meta."+key+"}", "is_set", "") {
			t.Fatalf("missing metadata-presence constraint for %s: %+v", key, job.Constraints)
		}
	}
	if !hasConstraint(job, "${attr.kernel.name}", "=", "darwin") || !hasConstraint(job, "${attr.cpu.arch}", "=", "arm64") {
		t.Fatalf("missing platform constraints: %+v", job.Constraints)
	}
	if job.Meta["kandev.node_id"] != "" {
		t.Fatal("pool job metadata records a node before placement")
	}
}

func TestPoolJobPinAddsNodeConstraint(t *testing.T) {
	p := poolTestProfile()
	p.PinNodeID = "node-abc"
	if !hasConstraint(buildPool(t, p), "${node.unique.id}", "=", "node-abc") {
		t.Fatal("pinned pool job lacks the node constraint")
	}
}

func TestPoolJobLocalExclusionAndAffinity(t *testing.T) {
	p := poolTestProfile()
	p.ExcludeLocal = true
	job := buildPool(t, p)
	if !hasConstraint(job, "${meta.kandev.attach}", "!=", "local") || len(job.Affinities) != 0 {
		t.Fatalf("exclude_local = %+v %+v", job.Constraints, job.Affinities)
	}
	p = poolTestProfile()
	p.PreferLocal = true
	job = buildPool(t, p)
	if len(job.Affinities) != 1 || job.Affinities[0].RTarget != "local" || job.Affinities[0].Weight <= 0 || hasConstraint(job, "${meta.kandev.attach}", "!=", "local") {
		t.Fatalf("prefer_local = %+v %+v", job.Constraints, job.Affinities)
	}
}

func TestPoolJobSpreadVersusBinpack(t *testing.T) {
	if got := buildPool(t, poolTestProfile()).Spreads; len(got) != 1 || got[0].Attribute != "${node.unique.id}" {
		t.Fatalf("spread placement spreads = %+v", got)
	}
	p := poolTestProfile()
	p.Placement = PlacementBinpack
	if got := buildPool(t, p).Spreads; len(got) != 0 {
		t.Fatalf("binpack placement carries spreads: %+v", got)
	}
}

func TestPoolJobKeepsSingleControllerFailClosedPolicy(t *testing.T) {
	g := buildPool(t, poolTestProfile()).TaskGroups[0]
	if g.Count != 1 || g.RestartPolicy.Attempts != 0 || g.ReschedulePolicy.Attempts != 0 || g.Disconnect.Replace || g.Disconnect.Reconcile != "keep_original" {
		t.Fatalf("pool job can restart or replace the worker process: %+v", g)
	}
	r := g.Tasks[0].Resources
	if r.CPU != 100 || r.MemoryMB != 256 || r.MemoryMaxMB != 2048 {
		t.Fatalf("resources = %+v", r)
	}
}

func TestPoolJobPathsResolveFromNodeMetadataAndMatchConcretePaths(t *testing.T) {
	job := buildPool(t, poolTestProfile())
	env := job.TaskGroups[0].Tasks[0].Env
	digest := strings.Repeat("c", 64)
	tmpl, err := PoolPathsFor(PoolStateRootExpr, job.ID, "env-1", 1, digest)
	if err != nil {
		t.Fatal(err)
	}
	if env["KANDEV_BOOTSTRAP_PATH"] != tmpl.BootstrapPath || env["KANDEV_AGENTCTL_PATH"] != tmpl.ArtifactPath ||
		env["KANDEV_WORKSPACE_ROOT"] != tmpl.WorkspacePath || env["KANDEV_READY_PATH"] != tmpl.ReadyPath {
		t.Fatalf("job env does not use the templated pool paths: %+v", env)
	}
	concrete, err := PoolPathsFor("/var/kandev", job.ID, "env-1", 1, digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(tmpl.BootstrapPath, PoolStateRootExpr, "/var/kandev") != concrete.BootstrapPath {
		t.Fatalf("template %q does not expand to %q", tmpl.BootstrapPath, concrete.BootstrapPath)
	}
	for _, v := range env {
		if strings.Contains(v, "secret") {
			t.Fatal("job env carries secret material")
		}
	}
}

func TestPoolJobRejectsLegacyProfileAndMismatchedSnapshot(t *testing.T) {
	legacy := poolTestProfile()
	legacy.Mode = ModeLegacyPinned
	if _, err := BuildPoolJob(PoolJobInput{Profile: legacy, State: poolTestState(t), ArtifactDigest: strings.Repeat("c", 64)}); err == nil {
		t.Fatal("legacy profile built a pool job")
	}
	p := poolTestProfile()
	p.CPUMHz = 999
	if _, err := BuildPoolJob(PoolJobInput{Profile: p, State: poolTestState(t), ArtifactDigest: strings.Repeat("c", 64)}); err == nil {
		t.Fatal("profile diverging from the snapshot built a job")
	}
}

func TestPoolJobDigestIsStableAndRetainedDigestEnforced(t *testing.T) {
	p := poolTestProfile()
	job := buildPool(t, p)
	d1, err := NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	s := poolTestState(t)
	s.NativeJobSpecSHA256, s.NativeJobPolicyVersion = d1, PoolJobPolicyVersion
	if _, err := BuildPoolJob(PoolJobInput{Profile: p, State: s, ArtifactDigest: strings.Repeat("c", 64)}); err != nil {
		t.Fatalf("retained digest rejected: %v", err)
	}
	p.Placement = PlacementBinpack
	if _, err := BuildPoolJob(PoolJobInput{Profile: p, State: s, ArtifactDigest: strings.Repeat("c", 64)}); err == nil {
		t.Fatal("changed placement policy matched the retained digest")
	}
}
