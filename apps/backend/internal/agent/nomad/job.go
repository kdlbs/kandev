package nomad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The job model is deliberately limited to the Nomad API fields used by the
// native Kandev executor. It is not a general-purpose jobspec builder.
type Job struct {
	ID                string            `json:"ID"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"`
	Priority          int               `json:"Priority,omitempty"`
	AllAtOnce         bool              `json:"AllAtOnce,omitempty"`
	NodePool          string            `json:"NodePool,omitempty"`
	Stop              bool              `json:"Stop,omitempty"`
	ParentID          string            `json:"ParentID,omitempty"`
	Dispatched        bool              `json:"Dispatched,omitempty"`
	Payload           []byte            `json:"Payload,omitempty"`
	Status            string            `json:"Status,omitempty"`
	StatusDescription string            `json:"StatusDescription,omitempty"`
	Stable            bool              `json:"Stable,omitempty"`
	Version           uint64            `json:"Version,omitempty"`
	SubmitTime        int64             `json:"SubmitTime,omitempty"`
	CreateIndex       uint64            `json:"CreateIndex,omitempty"`
	ModifyIndex       uint64            `json:"ModifyIndex,omitempty"`
	JobModifyIndex    uint64            `json:"JobModifyIndex,omitempty"`
	NomadTokenID      string            `json:"NomadTokenID,omitempty"`
	Namespace         string            `json:"Namespace"`
	Region            string            `json:"Region"`
	Datacenters       []string          `json:"Datacenters"`
	Meta              map[string]string `json:"Meta"`
	Constraints       []Constraint      `json:"Constraints"`
	Affinities        []Affinity        `json:"Affinities,omitempty"`
	Spreads           []Spread          `json:"Spreads,omitempty"`
	TaskGroups        []TaskGroup       `json:"TaskGroups"`
}

// Affinity is a soft placement preference.
type Affinity struct {
	LTarget string `json:"LTarget"`
	Operand string `json:"Operand"`
	RTarget string `json:"RTarget"`
	Weight  int    `json:"Weight"`
}

// Spread asks the scheduler to distribute allocations across an attribute.
type Spread struct {
	Attribute string `json:"Attribute"`
	Weight    int    `json:"Weight"`
}

type Constraint struct {
	LTarget string `json:"LTarget"`
	Operand string `json:"Operand"`
	RTarget string `json:"RTarget"`
}

type TaskGroup struct {
	Name             string            `json:"Name"`
	Count            int               `json:"Count"`
	RestartPolicy    RestartPolicy     `json:"RestartPolicy"`
	ReschedulePolicy ReschedulePolicy  `json:"ReschedulePolicy"`
	Disconnect       DisconnectPolicy  `json:"Disconnect"`
	Networks         []NetworkResource `json:"Networks"`
	Tasks            []Task            `json:"Tasks"`
}

type RestartPolicy struct {
	Attempts int    `json:"Attempts"`
	Interval int64  `json:"Interval"`
	Delay    int64  `json:"Delay"`
	Mode     string `json:"Mode"`
}

type ReschedulePolicy struct {
	Attempts      int    `json:"Attempts"`
	Interval      int64  `json:"Interval"`
	Delay         int64  `json:"Delay"`
	DelayFunction string `json:"DelayFunction"`
	MaxDelay      int64  `json:"MaxDelay"`
	Unlimited     bool   `json:"Unlimited"`
}

type DisconnectPolicy struct {
	Replace   bool   `json:"Replace"`
	Reconcile string `json:"Reconcile"`
	LostAfter int64  `json:"LostAfter"`
}

type NetworkResource struct {
	Mode         string `json:"Mode"`
	DynamicPorts []Port `json:"DynamicPorts"`
}

type Port struct {
	Label string `json:"Label"`
	Value int    `json:"Value"`
	To    int    `json:"To"`
}

type Task struct {
	Name        string            `json:"Name"`
	Driver      string            `json:"Driver"`
	Config      map[string]any    `json:"Config"`
	Env         map[string]string `json:"Env"`
	KillTimeout int64             `json:"KillTimeout"`
	Resources   TaskResources     `json:"Resources"`
}

type TaskResources struct {
	CPU         int `json:"CPU"`
	MemoryMB    int `json:"MemoryMB"`
	MemoryMaxMB int `json:"MemoryMaxMB,omitempty"`
}

type JobRegisterRequest struct {
	Job            Job    `json:"Job"`
	EnforceIndex   bool   `json:"EnforceIndex"`
	JobModifyIndex uint64 `json:"JobModifyIndex"`
}

type NativeJobInput struct {
	Profile       ProfileConfig
	State         EnvironmentState
	AgentctlPath  string
	BootstrapPath string
}

const defaultNodePool = "default"

func NativeJobFromState(state EnvironmentState) (Job, error) {
	if state.PlacementMode == ModePool {
		return BuildPoolJob(PoolJobInput{Profile: PoolProfileFromState(state), State: state, ArtifactDigest: state.ArtifactSHA256})
	}
	profile := ProfileConfig{
		Namespace: state.Namespace, Region: state.Region, Datacenter: state.Datacenter, NodeID: state.NodeID,
		Platform: state.Platform, SSHHost: state.SSHHost, SSHUser: state.SSHUser, SSHPort: state.SSHPort,
		SSHHostFingerprint: state.SSHHostFingerprint, WorkspaceRoot: state.WorkspaceRoot,
		AllocationRoot: state.AllocationRoot, CPUMHz: state.CPUMHz, MemoryMB: state.MemoryMB,
		DisconnectLostAfter: time.Duration(state.DisconnectLostAfterSeconds) * time.Second,
	}
	return BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
}

// BuildNativeJob returns a fixed-policy, one-controller Nomad job. The API
// registration wrapper must submit it with EnforceIndex=true and index zero.
func BuildNativeJob(input NativeJobInput) (Job, error) {
	p := input.Profile
	s := input.State
	if err := s.Validate(); err != nil {
		return Job{}, fmt.Errorf("build native Nomad job: invalid environment state: %w", err)
	}
	agentctlPath, bootstrapPath, err := validateNativeJobInput(input)
	if err != nil {
		return Job{}, err
	}
	metadata := nativeJobMetadata(s)
	group := nativeTaskGroup(p, s, agentctlPath, bootstrapPath)
	job := Job{
		ID: s.JobID, Name: s.JobID, Type: "batch", Namespace: p.Namespace, Region: p.Region,
		Priority: 50, NodePool: defaultNodePool, Datacenters: []string{p.Datacenter}, Meta: metadata,
		Constraints: []Constraint{{LTarget: nodeUniqueIDTarget, Operand: "=", RTarget: p.NodeID}},
		TaskGroups:  []TaskGroup{group},
	}
	if err := applyRetainedDigest(&job, s); err != nil {
		return Job{}, err
	}
	return job, nil
}

// validateNativeJobInput checks the profile, state and artifact paths against
// the immutable identity and returns the normalized artifact paths.
func validateNativeJobInput(input NativeJobInput) (string, string, error) {
	p := input.Profile
	s := input.State
	expectedWorkspacePath, workspaceErr := EnvironmentWorkspacePath(p.WorkspaceRoot, s.EnvironmentID, s.Generation)
	if workspaceErr != nil || expectedWorkspacePath != s.WorkspacePath || s.WorkspaceRoot != p.WorkspaceRoot {
		return "", "", fmt.Errorf("build native Nomad job: workspace path does not match immutable environment identity")
	}
	if !profileMatchesState(p, s) {
		return "", "", fmt.Errorf("build native Nomad job: profile does not match the immutable worker snapshot")
	}
	stableID, err := StableJobID(JobIdentity{
		EnvironmentID: s.EnvironmentID, Generation: s.Generation,
		OperationID: s.OperationID, InstallationID: s.InstallationID,
	})
	if err != nil || stableID != s.JobID {
		return "", "", fmt.Errorf("build native Nomad job: stable job ID does not match immutable identity")
	}
	agentctlPath, err := normalizedAbsolutePath(input.AgentctlPath)
	if err != nil {
		return "", "", fmt.Errorf("build native Nomad job: agentctl path: %w", err)
	}
	bootstrapPath, err := normalizedAbsolutePath(input.BootstrapPath)
	if err != nil {
		return "", "", fmt.Errorf("build native Nomad job: bootstrap path: %w", err)
	}
	if !strings.HasPrefix(agentctlPath, p.AllocationRoot+string(filepath.Separator)) ||
		!strings.HasPrefix(bootstrapPath, p.AllocationRoot+string(filepath.Separator)) {
		return "", "", fmt.Errorf("build native Nomad job: artifact and bootstrap paths must be inside the admitted allocation root")
	}
	if s.AgentctlPath != "" && s.AgentctlPath != agentctlPath {
		return "", "", fmt.Errorf("build native Nomad job: agentctl path differs from retained immutable evidence")
	}
	if s.BootstrapPath != "" && s.BootstrapPath != bootstrapPath {
		return "", "", fmt.Errorf("build native Nomad job: bootstrap path differs from retained immutable evidence")
	}
	return agentctlPath, bootstrapPath, nil
}

func profileMatchesState(p ProfileConfig, s EnvironmentState) bool {
	return p.NodeID == s.NodeID && p.Datacenter == s.Datacenter && p.Namespace == s.Namespace && p.Region == s.Region &&
		p.Platform == s.Platform && p.SSHHost == s.SSHHost && p.SSHUser == s.SSHUser && p.SSHPort == s.SSHPort &&
		p.SSHHostFingerprint == s.SSHHostFingerprint &&
		p.AllocationRoot == s.AllocationRoot && p.CPUMHz == s.CPUMHz && p.MemoryMB == s.MemoryMB &&
		profileMatchesDisconnectWindow(p, s)
}

// profileMatchesDisconnectWindow compares the profile with the immutable
// window snapshot. A retained environment from before the window was
// configurable carries zero and keeps its original job policy.
func profileMatchesDisconnectWindow(p ProfileConfig, s EnvironmentState) bool {
	return s.DisconnectLostAfterSeconds == 0 || p.DisconnectLostAfter == time.Duration(s.DisconnectLostAfterSeconds)*time.Second
}

func nativeJobMetadata(s EnvironmentState) map[string]string {
	return map[string]string{
		"kandev.environment_id":            s.EnvironmentID,
		"kandev.generation":                strconv.FormatInt(s.Generation, 10),
		"kandev.operation_id":              s.OperationID,
		"kandev.installation_id":           s.InstallationID,
		"kandev.input_digest":              s.InputDigest,
		"kandev.node_id":                   s.NodeID,
		"kandev.native_job_policy_version": strconv.Itoa(NativeJobPolicyVersion),
	}
}

func nativeTaskGroup(p ProfileConfig, s EnvironmentState, agentctlPath, bootstrapPath string) TaskGroup {
	return TaskGroup{
		Name: "agentctl", Count: 1,
		RestartPolicy: RestartPolicy{Attempts: 0, Interval: int64(time.Minute), Mode: "fail"},
		ReschedulePolicy: ReschedulePolicy{
			Attempts: 0, Interval: int64(24 * time.Hour), Delay: int64(5 * time.Second),
			DelayFunction: "constant", Unlimited: false,
		},
		Disconnect: DisconnectPolicy{Replace: false, Reconcile: "keep_original", LostAfter: int64(time.Duration(s.DisconnectLostAfterSeconds) * time.Second)},
		Networks: []NetworkResource{{Mode: "host", DynamicPorts: []Port{
			{Label: "agentctl_control", Value: 0},
			{Label: "agentctl_instance", Value: 0},
		}}},
		Tasks: []Task{{
			Name: "agentctl", Driver: "raw_exec",
			Config: map[string]any{"command": "/bin/sh", "args": []string{bootstrapPath}},
			Env: map[string]string{
				"KANDEV_AGENTCTL_PATH":          agentctlPath,
				"KANDEV_BOOTSTRAP_ENV_FILE":     bootstrapPath + ".env",
				"KANDEV_WORKSPACE_ROOT":         s.WorkspacePath,
				"KANDEV_ALLOCATION_ROOT":        p.AllocationRoot,
				"KANDEV_WORKER_STATE_ROOT":      filepath.Join(filepath.Dir(filepath.Dir(p.AllocationRoot)), "worker-state", p.NodeID),
				"KANDEV_NOMAD_NODE_ID":          nodeUniqueIDTarget,
				"KANDEV_NOMAD_JOB_ID":           s.JobID,
				"KANDEV_ENVIRONMENT_ID":         s.EnvironmentID,
				"KANDEV_ENVIRONMENT_GENERATION": strconv.FormatInt(s.Generation, 10),
				"KANDEV_OPERATION_ID":           s.OperationID,
				"KANDEV_INSTALLATION_ID":        s.InstallationID,
				"KANDEV_INPUT_DIGEST":           s.InputDigest,
				"AGENTCTL_LISTEN_HOST":          loopbackIPv4,
			},
			KillTimeout: int64(45 * time.Second),
			Resources:   TaskResources{CPU: p.CPUMHz, MemoryMB: p.MemoryMB},
		}},
	}
}

// applyRetainedDigest verifies and records the retained policy digest, when
// the state carries one.
func applyRetainedDigest(job *Job, s EnvironmentState) error {
	if s.NativeJobSpecSHA256 != "" {
		if s.NativeJobPolicyVersion != s.expectedJobPolicyVersion() {
			return fmt.Errorf("build native Nomad job: unsupported retained policy version")
		}
		digest, err := NativeJobSpecDigest(*job)
		if err != nil || digest != s.NativeJobSpecSHA256 {
			return fmt.Errorf("build native Nomad job: retained immutable policy digest does not match the expected job")
		}
		job.Meta["kandev.native_job_spec_sha256"] = digest
	}
	return nil
}

// NativeJobSpecDigest hashes the complete outbound fixed-policy job with a
// domain separator. The self-referential digest metadata is excluded.
func NativeJobSpecDigest(job Job) (string, error) {
	copy := job
	copy.Status = ""
	copy.StatusDescription = ""
	copy.Stable = false
	copy.Version = 0
	copy.SubmitTime = 0
	copy.CreateIndex = 0
	copy.ModifyIndex = 0
	copy.JobModifyIndex = 0
	copy.NomadTokenID = ""
	copy.Meta = make(map[string]string, len(job.Meta))
	for key, value := range job.Meta {
		if key != "kandev.native_job_spec_sha256" {
			copy.Meta[key] = value
		}
	}
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", fmt.Errorf("encode native Nomad job policy: %w", err)
	}
	digest := sha256.Sum256(append([]byte("kandev.nomad-native-job.v1\x00"), encoded...))
	return hex.EncodeToString(digest[:]), nil
}

func normalizedAbsolutePath(value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", fmt.Errorf("must be a normalized absolute path")
	}
	return value, nil
}
