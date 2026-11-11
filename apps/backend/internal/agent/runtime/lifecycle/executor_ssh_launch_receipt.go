package lifecycle

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// expectedSSHLaunchOwner builds the owner binding a persisted runtime record
// implies. The controller instance ID comes from the persisted metadata, not
// from the current local execution ID: a same-controller resume replaces the
// local execution while the remote controller keeps its original identity.
func expectedSSHLaunchOwner(metadata map[string]interface{}, taskID, sessionID string) (sshlaunch.Owner, bool) {
	owner := sshlaunch.Owner{
		TaskID:               strings.TrimSpace(taskID),
		SessionID:            strings.TrimSpace(sessionID),
		ControllerInstanceID: strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHAgentctlInstanceID)),
		TaskDir:              getMetadataString(metadata, MetadataKeySSHRemoteTaskDir),
		SessionDir:           getMetadataString(metadata, MetadataKeySSHRemoteSessionDir),
		Target: sshlaunch.TargetBinding{
			Host:            strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHHost)),
			Port:            strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHPort)),
			User:            strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHUser)),
			HostFingerprint: strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHHostFingerprint)),
		},
	}
	if err := owner.Validate(); err != nil {
		return sshlaunch.Owner{}, false
	}
	return owner, true
}

// persistedSSHLaunchReceipt returns the complete receipt a runtime record
// carries for its own owner. A missing key is legacy evidence and yields
// (nil, nil). A present but malformed or foreign receipt is an error: it is
// never downgraded to "no receipt", because that would let a conflicting
// record fall back to a weaker identity rule.
func persistedSSHLaunchReceipt(
	metadata map[string]interface{},
	taskID, sessionID string,
) (*sshlaunch.Receipt, error) {
	raw, present := metadata[sshlaunch.MetadataKey]
	if !present {
		return nil, nil
	}
	if raw == nil {
		// A present null is a malformed receipt, not absent legacy evidence.
		return nil, fmt.Errorf("%w: stored receipt is null", sshlaunch.ErrIncompleteReceipt)
	}
	receipt, ok := sshlaunch.ReceiptFromMetadata(metadata)
	if !ok {
		return nil, fmt.Errorf("%w: stored receipt is malformed", sshlaunch.ErrIncompleteReceipt)
	}
	owner, ok := expectedSSHLaunchOwner(metadata, taskID, sessionID)
	if !ok {
		return nil, fmt.Errorf("%w: stored runtime row is incomplete", sshlaunch.ErrIncompleteReceipt)
	}
	if err := receipt.ValidateFor(owner); err != nil {
		return nil, err
	}
	copied := receipt
	return &copied, nil
}

// sshLaunchReceiptForRecord is the recovery-side projection: a record with a
// present receipt must prove the exact owner, while a legacy record without
// one remains recoverable through the existing guarded path.
func sshLaunchReceiptForRecord(recordMetadata map[string]interface{}, taskID, sessionID string) (*sshlaunch.Receipt, bool) {
	receipt, err := persistedSSHLaunchReceipt(recordMetadata, taskID, sessionID)
	if err != nil {
		return nil, false
	}
	return receipt, true
}

// sshReconcileRequest builds the private helper request for one recorded
// controller. The receipt travels intact; the helper re-derives every other
// fact from the live host.
func sshReconcileRequest(receipt sshlaunch.Receipt) sshlaunch.ReconcileRequest {
	return sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version,
		Mode:    sshlaunch.ReconcileModeTerminate,
		Receipt: receipt,
	}
}

// sshReconcileVerifyRequest asks the helper to re-prove the live process
// against the receipt without signalling it. The caller keeps its own guarded
// stop for the present-directory case.
func sshReconcileVerifyRequest(receipt sshlaunch.Receipt) sshlaunch.ReconcileRequest {
	return sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version,
		Mode:    sshlaunch.ReconcileModeVerify,
		Receipt: receipt,
	}
}
