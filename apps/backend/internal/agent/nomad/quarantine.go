package nomad

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	QuarantinePending = "pending"
	QuarantineMarked  = "marked"
)

const quarantineMarkerItem = "marker"

// NodeRecord is the subset of a Nomad node needed to prove a dedicated worker.
type NodeRecord struct {
	ID                    string                `json:"ID"`
	Datacenter            string                `json:"Datacenter"`
	Status                string                `json:"Status"`
	SchedulingEligibility string                `json:"SchedulingEligibility"`
	Attributes            map[string]string     `json:"Attributes"`
	Meta                  map[string]string     `json:"Meta"`
	NodePool              string                `json:"NodePool"`
	ReservedResources     NodeReservedResources `json:"ReservedResources"`
	NodeResources         NodeTotalResources    `json:"NodeResources"`
}

// NodeTotalResources is the CPU and memory a node's client fingerprinted.
type NodeTotalResources struct {
	CPU struct {
		CPUShares int `json:"CpuShares"`
	} `json:"Cpu"`
	Memory struct {
		MemoryMB int `json:"MemoryMB"`
	} `json:"Memory"`
}

// NodeReservedResources is the client-reserved CPU and memory of a node.
type NodeReservedResources struct {
	CPU struct {
		CPUShares int `json:"CpuShares"`
	} `json:"Cpu"`
	Memory struct {
		MemoryMB int `json:"MemoryMB"`
	} `json:"Memory"`
}

// QuarantineMarker records why a dedicated node must not receive new work.
type QuarantineMarker struct {
	Version       int    `json:"version"`
	NodeID        string `json:"node_id"`
	EnvironmentID string `json:"environment_id"`
	OperationID   string `json:"operation_id"`
	JobID         string `json:"job_id"`
	Reason        string `json:"reason"`
	Generation    int64  `json:"generation"`
}

type quarantineVariable struct {
	Path        string            `json:"Path,omitempty"`
	Namespace   string            `json:"Namespace,omitempty"`
	ModifyIndex uint64            `json:"ModifyIndex,omitempty"`
	Items       map[string]string `json:"Items"`
}

// QuarantineVariablePath returns the Nomad variable path owning a node's marker.
func QuarantineVariablePath(nodeID string) string {
	if !safeNodeID.MatchString(nodeID) || nodeID == "." || nodeID == ".." {
		return ""
	}
	return "kandev/quarantine/" + nodeID
}

func quarantineVariableURL(namespace, nodeID, extra string) (string, error) {
	path := QuarantineVariablePath(nodeID)
	if path == "" || namespace == "" {
		return "", errors.New("nomad quarantine requires a namespace and safe node ID")
	}
	return "/v1/var/" + path + "?namespace=" + url.QueryEscape(namespace) + extra, nil
}

// GetNode loads one Nomad node record.
func (c *Client) GetNode(ctx context.Context, nodeID string) (*NodeRecord, error) {
	if !safeNodeID.MatchString(nodeID) {
		return nil, errors.New("nomad node lookup requires a safe node ID")
	}
	var node NodeRecord
	if err := c.getJSON(ctx, "/v1/node/"+url.PathEscape(nodeID), &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// SetNodeIneligible stops new placements on the node. Re-enabling placement is
// an operator-only action and intentionally has no client method.
func (c *Client) SetNodeIneligible(ctx context.Context, nodeID string) error {
	if !safeNodeID.MatchString(nodeID) {
		return errors.New("nomad node eligibility requires a safe node ID")
	}
	payload, err := json.Marshal(map[string]string{"NodeID": nodeID, "Eligibility": "ineligible"})
	if err != nil {
		return fmt.Errorf("encode Nomad node eligibility: %w", err)
	}
	status, _, err := c.doRaw(ctx, http.MethodPost, "/v1/node/"+url.PathEscape(nodeID)+"/eligibility", payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("nomad node eligibility returned HTTP %d", status)
	}
	return nil
}

// CheckDedicatedNode verifies the live node is the profile's approved worker.
func CheckDedicatedNode(profile ProfileConfig, node *NodeRecord) error {
	if node == nil || node.ID != profile.NodeID || node.Datacenter != profile.Datacenter {
		return errors.New("nomad node does not match the dedicated worker identity")
	}
	kernel, arch, ok := strings.Cut(profile.Platform, "/")
	if !ok || node.Attributes["kernel.name"] != kernel || normalizeArch(node.Attributes["cpu.arch"]) != normalizeArch(arch) || arch == "" {
		return errors.New("nomad node platform does not match the dedicated worker profile")
	}
	return nil
}

func normalizeArch(arch string) string {
	switch arch {
	case "aarch64", archARM64:
		return archARM64
	case "x86_64", "amd64":
		return "amd64"
	}
	return arch
}

// ReadQuarantineMarker returns the node marker and its ModifyIndex, or nil when absent.
func (c *Client) ReadQuarantineMarker(ctx context.Context, namespace, nodeID string) (*QuarantineMarker, uint64, error) {
	path, err := quarantineVariableURL(namespace, nodeID, "")
	if err != nil {
		return nil, 0, err
	}
	status, body, err := c.doRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusNotFound {
		return nil, 0, nil
	}
	if status != http.StatusOK {
		return nil, 0, fmt.Errorf("nomad quarantine marker read returned HTTP %d", status)
	}
	marker, index, err := decodeQuarantineVariable(body)
	if err != nil {
		return nil, 0, err
	}
	return marker, index, nil
}

// PutQuarantineMarkerCAS creates the marker only when none exists. An existing
// marker is returned unchanged and is never overwritten or deleted.
func (c *Client) PutQuarantineMarkerCAS(ctx context.Context, namespace string, m QuarantineMarker) (*QuarantineMarker, bool, error) {
	path, err := quarantineVariableURL(namespace, m.NodeID, "&cas=0")
	if err != nil {
		return nil, false, err
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return nil, false, fmt.Errorf("encode Nomad quarantine marker: %w", err)
	}
	payload, err := json.Marshal(quarantineVariable{Path: QuarantineVariablePath(m.NodeID), Namespace: namespace, Items: map[string]string{quarantineMarkerItem: string(encoded)}})
	if err != nil {
		return nil, false, fmt.Errorf("encode Nomad quarantine variable: %w", err)
	}
	status, body, err := c.doRaw(ctx, http.MethodPut, path, payload)
	if err != nil {
		return nil, false, err
	}
	if status >= 200 && status < 300 {
		return nil, true, nil
	}
	if status != http.StatusConflict {
		return nil, false, fmt.Errorf("nomad quarantine marker write returned HTTP %d", status)
	}
	if existing, _, err := decodeQuarantineVariable(body); err == nil && existing != nil {
		return existing, false, nil
	}
	existing, _, err := c.ReadQuarantineMarker(ctx, namespace, m.NodeID)
	if err != nil || existing == nil {
		return nil, false, errors.New("nomad quarantine marker conflict could not be resolved")
	}
	return existing, false, nil
}

func decodeQuarantineVariable(body []byte) (*QuarantineMarker, uint64, error) {
	var variable quarantineVariable
	if err := json.Unmarshal(body, &variable); err != nil {
		return nil, 0, errors.New("decode Nomad quarantine variable failed")
	}
	raw, ok := variable.Items[quarantineMarkerItem]
	if !ok {
		return nil, 0, errors.New("nomad quarantine variable has no marker")
	}
	var marker QuarantineMarker
	if err := json.Unmarshal([]byte(raw), &marker); err != nil {
		return nil, 0, errors.New("decode Nomad quarantine marker failed")
	}
	return &marker, variable.ModifyIndex, nil
}

// doRaw sends one authenticated request and returns the status and bounded body.
func (c *Client) doRaw(ctx context.Context, method, path string, payload []byte) (int, []byte, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build Nomad API request: %w", err)
	}
	req.Header.Set("X-Nomad-Token", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("nomad API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJobResponseBytes+1))
	if err != nil || len(body) > maxJobResponseBytes {
		return 0, nil, errors.New("read Nomad API response failed")
	}
	return resp.StatusCode, body, nil
}
