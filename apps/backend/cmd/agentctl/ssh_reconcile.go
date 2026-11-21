package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kandev/kandev/internal/agentctl/sshreconcile"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// maxSSHReconcileRequestBytes bounds the request body accepted from the
// backend. The receipt is small; anything larger is not a valid request.
const maxSSHReconcileRequestBytes = 64 * 1024

// runSSHReconcile is the private `agentctl ssh-reconcile` utility invoked over
// SSH before server startup. It reads one JSON request from stdin and writes
// one JSON result to stdout. Every outcome is a result; a non-zero exit is
// reserved for an unwritable response.
func runSSHReconcile(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	body, err := io.ReadAll(io.LimitReader(stdin, maxSSHReconcileRequestBytes+1))
	if err != nil || len(body) > maxSSHReconcileRequestBytes {
		return writeSSHReconcileResult(stdout, sshlaunch.Unresolved(
			sshlaunch.ReasonProbeUnknown, "reconcile request could not be read"))
	}
	var request sshlaunch.ReconcileRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return writeSSHReconcileResult(stdout, sshlaunch.Unresolved(
			sshlaunch.ReasonProbeUnknown, "reconcile request is malformed"))
	}
	return writeSSHReconcileResult(stdout, sshreconcile.Reconcile(ctx, request))
}

func writeSSHReconcileResult(stdout io.Writer, result sshlaunch.ReconcileResult) int {
	encoded, err := json.Marshal(result)
	if err != nil {
		return 1
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return 1
	}
	return 0
}
