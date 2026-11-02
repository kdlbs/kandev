package sqlite

import (
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
)

func launchFactsMatchSnapshot(facts *contract.ExecutionFacts, snapshot *models.ExecutionSnapshot) bool {
	if facts == nil || facts.LaunchChecks.Receipt == nil {
		return true
	}
	receipt := facts.LaunchChecks.Receipt
	captured := snapshot.Sandbox
	return receipt.PolicyDigest == captured.PolicyDigest && receipt.PackageDigest == captured.PackageDigest && receipt.RuntimeDigest == captured.RuntimeDigest
}

func preserveExecutionHistory(value, previous *models.ExecutionObservation) {
	if previous.Facts == nil {
		return
	}
	if value.Facts == nil {
		value.Facts = &contract.ExecutionFacts{SchemaVersion: 1, Process: contract.ProcessFact{State: contract.StatusUnknown}, LaunchChecks: previous.Facts.LaunchChecks, Runtime: contract.RuntimeFacts{State: contract.FactUnavailable}, Audit: previous.Facts.Audit}
		if value.Facts.Audit.Signer == "trusted" {
			value.Facts.Audit.Signer = "untrusted"
		}
	}
	if len(value.Facts.Runtime.Observations) == 0 && len(previous.Facts.Runtime.Observations) > 0 {
		value.Facts.Runtime.Observations = previous.Facts.Runtime.Observations
		value.Facts.Runtime.State = contract.FactUnavailable
	}
}
