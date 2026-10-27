package nomad

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrRegistrationOutcomeUnknown means the request may have reached Nomad but
// the caller did not receive a definitive response. Callers must reconcile the
// stable immutable job ID before trying again; they must not create a new ID.
var ErrRegistrationOutcomeUnknown = errors.New("nomad job registration outcome is uncertain")
var ErrJobRegistrationConflict = errors.New("nomad job registration conflicts with an existing stable job")

type RegisterResult struct {
	EvalID         string `json:"EvalID"`
	JobModifyIndex uint64 `json:"JobModifyIndex"`
}

type JobRecord struct {
	ID             string            `json:"ID"`
	Namespace      string            `json:"Namespace"`
	Region         string            `json:"Region"`
	Meta           map[string]string `json:"Meta"`
	CreateIndex    uint64            `json:"CreateIndex"`
	ModifyIndex    uint64            `json:"ModifyIndex"`
	JobModifyIndex uint64            `json:"JobModifyIndex"`
	Spec           Job               `json:"-"`
	Raw            json.RawMessage   `json:"-"`
}

type AllocationTaskState struct {
	State string `json:"State"`
}

type AllocationPort struct {
	Label  string `json:"Label"`
	Value  int    `json:"Value"`
	To     int    `json:"To"`
	HostIP string `json:"HostIP"`
}

type AllocationSharedResources struct {
	Ports []AllocationPort `json:"Ports"`
}

type AllocationResources struct {
	Shared AllocationSharedResources `json:"Shared"`
}

type AllocationRecord struct {
	ID                 string                         `json:"ID"`
	NodeID             string                         `json:"NodeID"`
	JobID              string                         `json:"JobID"`
	Namespace          string                         `json:"Namespace"`
	DesiredStatus      string                         `json:"DesiredStatus"`
	ClientStatus       string                         `json:"ClientStatus"`
	CreateIndex        uint64                         `json:"CreateIndex"`
	TaskStates         map[string]AllocationTaskState `json:"TaskStates"`
	AllocatedResources *AllocationResources           `json:"AllocatedResources"`
	Job                json.RawMessage                `json:"Job"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// CloseIdleConnections closes reusable HTTP connections after the caller is done with this client.
func (c *Client) CloseIdleConnections() {
	if c != nil && c.http != nil {
		c.http.CloseIdleConnections()
	}
}

const (
	loopbackIPv4 = "127.0.0.1"
	schemeHTTPS  = "https"
)

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == loopbackIPv4 || host == "::1"
}

// ClientOption customizes NewClient.
type ClientOption func(*clientOptions)

type clientOptions struct{ caFile string }

// WithCAFile makes the client trust the PEM CA bundle at path, instead of the
// system trust store, for the Nomad API. Hostname verification stays on.
func WithCAFile(path string) ClientOption {
	return func(o *clientOptions) { o.caFile = path }
}

// NewClientForProfile builds the API client a profile describes.
func NewClientForProfile(profile ProfileConfig, token string) (*Client, error) {
	var opts []ClientOption
	if profile.CAFile != "" {
		opts = append(opts, WithCAFile(profile.CAFile))
	}
	return NewClient(profile.Address, token, opts...)
}

func loadCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ca_file %q is not readable: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("ca_file %q contains no PEM certificate", path)
	}
	return pool, nil
}

func NewClient(rawAddress, token string, options ...ClientOption) (*Client, error) {
	var opts clientOptions
	for _, option := range options {
		option(&opts)
	}
	parsed, err := url.Parse(rawAddress)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path != "" {
		return nil, errors.New("nomad API address must be an origin URL without credentials, path, query, or fragment")
	}
	if parsed.Scheme != schemeHTTPS && (parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("nomad API requires HTTPS except for loopback development")
	}
	if token == "" {
		return nil, errors.New("nomad API token is required")
	}
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.caFile != "" {
		if parsed.Scheme != schemeHTTPS {
			return nil, errors.New("ca_file requires an https nomad address")
		}
		pool, err := loadCAPool(opts.caFile)
		if err != nil {
			return nil, err
		}
		httpTransport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	transport := &http.Client{
		Transport: httpTransport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{baseURL: strings.TrimRight(rawAddress, "/"), token: token, http: transport}, nil
}

func (c *Client) RegisterCreateOnly(ctx context.Context, job Job) (RegisterResult, error) {
	request := JobRegisterRequest{Job: job, EnforceIndex: true, JobModifyIndex: 0}
	payload, err := json.Marshal(request)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("encode Nomad job registration: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/v1/jobs", bytes.NewReader(payload))
	if err != nil {
		return RegisterResult{}, fmt.Errorf("build Nomad job registration: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nomad-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("%w: %v", ErrRegistrationOutcomeUnknown, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never include response bodies: Nomad diagnostics can echo submitted
		// jobspec fields, and the caller only needs a safe status classification.
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return RegisterResult{}, fmt.Errorf("%w: Nomad returned HTTP %d", ErrRegistrationOutcomeUnknown, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusConflict {
			return RegisterResult{}, ErrJobRegistrationConflict
		}
		return RegisterResult{}, fmt.Errorf("nomad job registration returned HTTP %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, 1<<20)
	var result RegisterResult
	if err := json.NewDecoder(limited).Decode(&result); err != nil || result.JobModifyIndex == 0 {
		return RegisterResult{}, fmt.Errorf("%w: invalid registration response", ErrRegistrationOutcomeUnknown)
	}
	return result, nil
}

func (c *Client) GetJob(ctx context.Context, namespace, jobID string) (*JobRecord, error) {
	if namespace == "" || jobID == "" {
		return nil, errors.New("nomad job lookup requires namespace and stable job ID")
	}
	path := "/v1/job/" + url.PathEscape(jobID) + "?namespace=" + url.QueryEscape(namespace)
	raw, err := c.getRawJSON(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, err
	}
	var result JobRecord
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode Nomad job identity: %w", err)
	}
	if err := json.Unmarshal(raw, &result.Spec); err != nil {
		return nil, fmt.Errorf("decode Nomad job policy: %w", err)
	}
	result.Raw = append(json.RawMessage(nil), raw...)
	return &result, nil
}

func (c *Client) ListAllocations(ctx context.Context, namespace, jobID string) ([]AllocationRecord, error) {
	if namespace == "" || jobID == "" {
		return nil, errors.New("nomad allocation lookup requires namespace and stable job ID")
	}
	var result []AllocationRecord
	path := "/v1/job/" + url.PathEscape(jobID) + "/allocations?all=true&namespace=" + url.QueryEscape(namespace)
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAllocation loads the full allocation record. Nomad's job allocation list
// is a summary and omits AllocatedResources, including the dynamically
// reserved port values needed to connect to the task.
func (c *Client) GetAllocation(ctx context.Context, namespace, allocationID string) (*AllocationRecord, error) {
	if namespace == "" || allocationID == "" {
		return nil, errors.New("nomad allocation lookup requires namespace and allocation ID")
	}
	var result AllocationRecord
	path := "/v1/allocation/" + url.PathEscape(allocationID) + "?namespace=" + url.QueryEscape(namespace)
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BindSingleRunningAllocation accepts only the single exact allocation the
// immutable job can own. Empty, multiple, non-running, or mismatched records
// stay unknown and cannot be used to open an SSH tunnel.
func BindSingleRunningAllocation(state EnvironmentState, records []AllocationRecord) (EnvironmentState, error) {
	if err := state.Validate(); err != nil {
		return state, fmt.Errorf("bind Nomad allocation: invalid state: %w", err)
	}
	if state.Phase != "registered" || state.AllocationID != "" || len(records) != 1 {
		return state, errors.New("bind Nomad allocation: exactly one new allocation is required")
	}
	allocation := records[0]
	if err := checkAllocationIdentity(state, allocation); err != nil {
		return state, err
	}
	controlPort, instancePort, err := reservedAgentctlPorts(allocation)
	if err != nil {
		return state, err
	}
	state.Revision++
	state.AllocationID = allocation.ID
	state.AllocationCreateIndex = allocation.CreateIndex
	state.ControlPort = controlPort
	state.InstancePort = instancePort
	state.Phase = "allocated"
	return state, nil
}

func (c *Client) getJSON(ctx context.Context, path string, result any) error {
	raw, err := c.getRawJSON(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode Nomad API response: %w", err)
	}
	return nil
}

func (c *Client) getRawJSON(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build Nomad API request: %w", err)
	}
	req.Header.Set("X-Nomad-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nomad API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nomad API returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJobResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Nomad API response: %w", err)
	}
	if len(raw) > maxJobResponseBytes {
		return nil, fmt.Errorf("nomad API response exceeds the size limit")
	}
	return raw, nil
}

func checkAllocationIdentity(state EnvironmentState, allocation AllocationRecord) error {
	if allocation.ID == "" || allocation.CreateIndex == 0 || allocation.JobID != state.JobID ||
		allocation.NodeID != state.NodeID || allocation.Namespace != state.Namespace ||
		allocation.DesiredStatus != "run" || allocation.ClientStatus != "running" {
		return errors.New("bind Nomad allocation: allocation identity or running state does not match")
	}
	if allocation.TaskStates["agentctl"].State != "running" {
		return errors.New("bind Nomad allocation: agentctl task is not running")
	}
	return nil
}

func reservedAgentctlPorts(allocation AllocationRecord) (int, int, error) {
	ports := map[string]int{}
	if allocation.AllocatedResources == nil {
		return 0, 0, errors.New("bind Nomad allocation: reserved ports are missing")
	}
	for _, port := range allocation.AllocatedResources.Shared.Ports {
		if port.Label == "agentctl_control" || port.Label == "agentctl_instance" {
			if port.Value < 1 || port.Value > 65535 || ports[port.Label] != 0 {
				return 0, 0, errors.New("bind Nomad allocation: reserved port identity is invalid")
			}
			ports[port.Label] = port.Value
		}
	}
	controlPort, instancePort := ports["agentctl_control"], ports["agentctl_instance"]
	if controlPort == 0 || instancePort == 0 || controlPort == instancePort {
		return 0, 0, errors.New("bind Nomad allocation: distinct reserved control and instance ports are required")
	}
	return controlPort, instancePort, nil
}
