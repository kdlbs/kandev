package api

import (
	"fmt"
	"os"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// buildLaunchReceipt captures this process's own identity for a managed SSH
// launch. The controller is the only party that can report the birth identity
// of the process the backend is actually about to adopt, so the receipt is
// produced here, inside the nonce-authenticated handshake, rather than
// reconstructed later from a pidfile or a command line.
//
// A failure is terminal for that launch attempt: a launch that cannot carry
// its own proof must not be published, because the alternative is a row that
// looks supportable but can never be reconciled after directory loss.
func buildLaunchReceipt(binding sshlaunch.Binding) (sshlaunch.Receipt, error) {
	if err := binding.Validate(); err != nil {
		return sshlaunch.Receipt{}, fmt.Errorf("invalid SSH launch binding: %w", err)
	}
	identity, err := processidentity.Capture(os.Getpid())
	if err != nil {
		return sshlaunch.Receipt{}, fmt.Errorf("capture controller process identity: %w", err)
	}
	receipt := sshlaunch.Receipt{
		Version:    sshlaunch.Version,
		Generation: binding.Generation,
		Owner:      binding.Owner,
		UID:        os.Geteuid(),
		Process:    identity,
	}
	if err := receipt.Validate(); err != nil {
		return sshlaunch.Receipt{}, fmt.Errorf("capture controller launch receipt: %w", err)
	}
	return receipt, nil
}
