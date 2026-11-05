package plugins

const (
	sandboxReferenceWorkDir      = "work_dir"
	sandboxReferenceAgentState   = "agent_state"
	sandboxTransportTCP          = "tcp"
	sandboxOperationCommand      = "command"
	sandboxOperationHTTPEndpoint = "http_endpoint"
	sandboxNetworkModeNone       = "none"
	sandboxPlatformLinux         = "linux"
	sandboxFinalizationFinal     = "final"
	sandboxEventAllowed          = "allowed"
)

const sandboxEventDenied = "denied"
const sandboxIntegrityInvalid = "invalid"

const (
	sandboxV2ApprovalDenied    = "approval_denied"
	sandboxV2ApprovalGranted   = "approval_granted"
	sandboxV2ApprovalRequested = "approval_requested"
	sandboxV2ChildProcess      = "child_process"
	sandboxV2Error             = "error"
	sandboxV2HTTPS             = "https"
	sandboxV2Interrupted       = "interrupted"
	sandboxV2Live              = "live"
	sandboxV2MacOS             = "macos"
	sandboxV2NetworkPreflight  = "network_preflight"
	sandboxV2ReadWrite         = "read_write"
	sandboxV2RuntimeService    = "runtime_service"
	sandboxV2StaticEgress      = "static_egress"
	sandboxV2Supported         = "supported"
	sandboxV2Unavailable       = "unavailable"
	sandboxV2Unchecked         = "unchecked"
	sandboxV2Unsigned          = "unsigned"
	sandboxV2Unsupported       = "unsupported"
	sandboxV2Untrusted         = "untrusted"
	sandboxV2Valid             = "valid"
	sandboxV2Verified          = "verified"
	sandboxV2Write             = "write"
)
