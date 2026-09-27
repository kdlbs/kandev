package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	guardedTTYCapabilityName   = "kandev.guarded-tty-exec"
	guardedTTYContractVersion  = streams.GuardedTTYContractVersion
	guardedTTYCapabilityMethod = "_kandev/guarded_tty/capability"
	guardedTTYExecMethod       = "_kandev/guarded_tty/exec"
	guardedTTYProbeTimeout     = 2 * time.Second
	guardedTTYOutcomeCompleted = "completed"
	guardedTTYOutcomeFailed    = "failed"
	guardedTTYOutcomeDenied    = "denied"
)

type guardedTTYAdvertisement struct {
	Capability       string `json:"capability"`
	Version          int    `json:"version"`
	CapabilityMethod string `json:"capabilityMethod"`
	ExecMethod       string `json:"execMethod"`
}

type guardedTTYCapabilityResponse struct {
	Capability       string `json:"capability"`
	Version          int    `json:"version"`
	Supported        bool   `json:"supported"`
	CapabilityMethod string `json:"capability_method"`
	ExecMethod       string `json:"exec_method"`
	SessionID        string `json:"session_id"`
}

type guardedTTYWireReceipt struct {
	Capability    string  `json:"capability"`
	Version       int     `json:"version"`
	SessionID     string  `json:"session_id"`
	Method        string  `json:"method"`
	RequestedTTY  bool    `json:"requested_tty"`
	DispatchedTTY bool    `json:"dispatched_tty"`
	ProcessID     *string `json:"process_id"`
	CWD           *string `json:"cwd"`
	Outcome       string  `json:"outcome"`
	DenialCode    *string `json:"denial_code"`
	Stdout        string  `json:"stdout"`
	Stderr        string  `json:"stderr"`
	StdoutBytes   int     `json:"stdout_bytes"`
	StderrBytes   int     `json:"stderr_bytes"`
	OutputBytes   int     `json:"output_bytes"`
	ExitCode      *int    `json:"exit_code"`
	StartedAt     string  `json:"started_at"`
	CompletedAt   string  `json:"completed_at"`
}

func guardedTTYAdvertisementIsExact(agentID string, meta map[string]any) bool {
	if agentID != codexAgentID || meta == nil {
		return false
	}
	rawAdvertisement, ok := meta["guardedTtyExec"]
	if !ok {
		return false
	}
	raw, err := json.Marshal(rawAdvertisement)
	if err != nil {
		return false
	}
	var advertisement guardedTTYAdvertisement
	if err := decodeExactJSON(raw, &advertisement); err != nil {
		return false
	}
	return advertisement.Capability == guardedTTYCapabilityName &&
		advertisement.Version == guardedTTYContractVersion &&
		advertisement.CapabilityMethod == guardedTTYCapabilityMethod &&
		advertisement.ExecMethod == guardedTTYExecMethod
}

// GuardedTTYAvailable reports exact initialize advertisement plus a successful
// probe bound to the adapter's current ACP session.
func (a *Adapter) GuardedTTYAvailable() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.guardedTTYAvailable
}

func (a *Adapter) refreshGuardedTTYAvailability(ctx context.Context, sessionID string) {
	a.mu.RLock()
	conn := a.acpConn
	advertised := a.guardedTTYAdvertised
	a.mu.RUnlock()
	available := false
	if conn != nil && advertised && sessionID != "" {
		probeCtx, cancel := context.WithTimeout(ctx, guardedTTYProbeTimeout)
		raw, err := conn.CallExtension(probeCtx, guardedTTYCapabilityMethod, map[string]any{
			"sessionId": sessionID,
		})
		cancel()
		if err == nil {
			var response guardedTTYCapabilityResponse
			if decodeErr := decodeExactJSON(raw, &response); decodeErr == nil {
				available = response.Capability == guardedTTYCapabilityName &&
					response.Version == guardedTTYContractVersion && response.Supported &&
					response.CapabilityMethod == guardedTTYCapabilityMethod &&
					response.ExecMethod == guardedTTYExecMethod && response.SessionID == sessionID
			}
		}
	}
	a.mu.Lock()
	// A session replacement that completed while the probe was in flight must
	// not inherit the predecessor's positive result.
	a.guardedTTYAvailable = available && a.sessionID == sessionID
	a.mu.Unlock()
}

// ExecuteGuardedTTY calls only the negotiated extension for the active ACP
// session. It forwards no cwd, environment, sandbox, permission, process, or
// terminal controls; codex-acp derives those from its guarded session state.
func (a *Adapter) ExecuteGuardedTTY(
	ctx context.Context,
	request streams.GuardedTTYBridgeRequest,
) (*streams.GuardedTTYExecReceipt, error) {
	if !streams.ValidateGuardedTTYArgv(request.Argv) {
		return nil, errors.New("guarded TTY argv is invalid")
	}
	a.mu.RLock()
	conn := a.acpConn
	activeSessionID := a.sessionID
	available := a.guardedTTYAvailable
	bridgeVersion := ""
	if a.agentInfo != nil {
		bridgeVersion = a.agentInfo.Version
	}
	a.mu.RUnlock()
	if conn == nil || !available || request.SessionID == "" || request.SessionID != activeSessionID {
		return nil, errors.New("guarded TTY bridge is unavailable")
	}
	raw, err := conn.CallExtension(ctx, guardedTTYExecMethod, map[string]any{
		"sessionId": request.SessionID,
		"argv":      append([]string(nil), request.Argv...),
	})
	if err != nil {
		return nil, errors.New("guarded TTY bridge request failed")
	}
	var wire guardedTTYWireReceipt
	if err := decodeExactJSON(raw, &wire); err != nil {
		return nil, errors.New("guarded TTY bridge returned an invalid receipt")
	}
	return mapGuardedTTYReceipt(wire, bridgeVersion)
}

func mapGuardedTTYReceipt(wire guardedTTYWireReceipt, bridgeVersion string) (*streams.GuardedTTYExecReceipt, error) {
	startedAt, completedAt, err := validateGuardedTTYWireReceipt(wire)
	if err != nil {
		return nil, err
	}
	outcome, err := normalizeGuardedTTYOutcome(wire)
	if err != nil {
		return nil, err
	}
	receipt := &streams.GuardedTTYExecReceipt{
		ContractVersion: guardedTTYContractVersion,
		BridgeVersion:   bridgeVersion,
		ACPSessionID:    wire.SessionID,
		Method:          wire.Method,
		RequestedTTY:    wire.RequestedTTY,
		DispatchedTTY:   wire.DispatchedTTY,
		Stdout:          wire.Stdout,
		Stderr:          wire.Stderr,
		StdoutBytes:     wire.StdoutBytes,
		StderrBytes:     wire.StderrBytes,
		Output:          wire.Stdout + wire.Stderr,
		OutputBytes:     wire.OutputBytes,
		Outcome:         outcome,
		CompletionCount: 1,
		StartedAt:       startedAt,
		CompletedAt:     completedAt,
	}
	if wire.ProcessID != nil {
		receipt.ProcessID = *wire.ProcessID
	}
	if wire.CWD != nil {
		receipt.CWD = *wire.CWD
	}
	if wire.ExitCode != nil {
		receipt.ExitCode = *wire.ExitCode
	}
	if wire.DenialCode != nil {
		receipt.DenialCode = *wire.DenialCode
	}
	return receipt, nil
}

func validateGuardedTTYWireReceipt(wire guardedTTYWireReceipt) (time.Time, time.Time, error) {
	startedAt, startErr := time.Parse(time.RFC3339Nano, wire.StartedAt)
	completedAt, completeErr := time.Parse(time.RFC3339Nano, wire.CompletedAt)
	if startErr != nil || completeErr != nil || completedAt.Before(startedAt) {
		return time.Time{}, time.Time{}, errors.New("guarded TTY bridge returned invalid timestamps")
	}
	if wire.Capability != guardedTTYCapabilityName || wire.Version != guardedTTYContractVersion ||
		wire.SessionID == "" || wire.Method != streams.GuardedTTYExecMethod || !wire.RequestedTTY {
		return time.Time{}, time.Time{}, errors.New("guarded TTY bridge returned an invalid receipt")
	}
	if wire.StdoutBytes != len(wire.Stdout) || wire.StderrBytes != len(wire.Stderr) {
		return time.Time{}, time.Time{}, errors.New("guarded TTY bridge returned invalid stream sizes")
	}
	if wire.OutputBytes != wire.StdoutBytes+wire.StderrBytes || wire.OutputBytes > streams.GuardedTTYMaxOutputBytes {
		return time.Time{}, time.Time{}, errors.New("guarded TTY bridge returned an invalid output size")
	}
	return startedAt, completedAt, nil
}

func normalizeGuardedTTYOutcome(wire guardedTTYWireReceipt) (string, error) {
	dispatchIdentityValid := wire.ProcessID != nil && *wire.ProcessID != "" && wire.CWD != nil && *wire.CWD != ""
	if wire.Outcome == guardedTTYOutcomeCompleted {
		if wire.DispatchedTTY && dispatchIdentityValid && wire.DenialCode == nil && wire.ExitCode != nil {
			return "succeeded", nil
		}
		return "", errors.New("guarded TTY bridge returned an invalid completed outcome")
	}
	return normalizeGuardedTTYDenial(wire, dispatchIdentityValid)
}

func normalizeGuardedTTYDenial(wire guardedTTYWireReceipt, dispatchIdentityValid bool) (string, error) {
	if (wire.Outcome != guardedTTYOutcomeFailed && wire.Outcome != guardedTTYOutcomeDenied) ||
		wire.DenialCode == nil || wire.ExitCode != nil {
		return "", errors.New("guarded TTY bridge returned an invalid outcome")
	}
	if (wire.Outcome == guardedTTYOutcomeFailed) != wire.DispatchedTTY || wire.DispatchedTTY != dispatchIdentityValid {
		return "", errors.New("guarded TTY bridge returned inconsistent dispatch evidence")
	}
	if knownGuardedTTYDenial(*wire.DenialCode) {
		return *wire.DenialCode, nil
	}
	return "", fmt.Errorf("guarded TTY bridge returned an unknown denial")
}

func knownGuardedTTYDenial(code string) bool {
	switch code {
	case streams.GuardedTTYDenialAppServer, streams.GuardedTTYDenialCancelled,
		streams.GuardedTTYDenialInvalid, streams.GuardedTTYDenialOverflow,
		streams.GuardedTTYDenialStale, streams.GuardedTTYDenialTimeout:
		return true
	default:
		return false
	}
}

func decodeExactJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}
