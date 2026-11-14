package acpcompat

// CopilotNativeCompactionCapability is the optional agentctl peer capability
// for Copilot's opt-in native compaction and attributed usage stream. It is not
// a required survival capability; older retained peers remain adoptable and
// report native counting unavailable.
const CopilotNativeCompactionCapability = "copilot-native-compaction.v1"
