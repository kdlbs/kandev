package models

// RemoteRecoveryOwnerLockIDs identify the durable rows that authorize a
// remote-recovery disposition. Repository writers lock these rows before
// validating the owner and changing executor inventory.
type RemoteRecoveryOwnerLockIDs struct {
	TaskID                 string
	SessionID              string
	WorkspaceID            string
	EnvironmentID          string
	EnvironmentOwnerTaskID string
}
