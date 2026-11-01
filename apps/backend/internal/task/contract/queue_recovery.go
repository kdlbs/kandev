package contract

// MaxQueueRecoveryResponseBytes bounds each serialized recovery response,
// including its WebSocket message or MCP tool-result wrapper. Transport framing
// outside those response objects is governed by the transport's own limits.
const MaxQueueRecoveryResponseBytes = 2 * 1024 * 1024
