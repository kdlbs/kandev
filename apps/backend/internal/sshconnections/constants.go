package sshconnections

const (
	identityFile                  = "file"
	sshHostConfigKey              = "ssh_host"
	codeHostKeyChanged            = "ssh_host_key_changed"
	codeAgentUnavailable          = "ssh_agent_unavailable"
	codeKeyFileUnavailable        = "ssh_key_file_unavailable"
	enrollmentStarted             = "started"
	enrollmentPassword            = "password"
	hopBastion                    = "bastion"
	hopDestination                = "destination"
	enrollmentInstalledUnverified = "installed_unverified"
	errorResponseKey              = "error"
	identityManaged               = "managed"
	identityAgent                 = "agent"
	identityGenerate              = "generate"
	identityImport                = "import"
	codeManagedKeyUnavailable     = "ssh_managed_key_unavailable"
)
