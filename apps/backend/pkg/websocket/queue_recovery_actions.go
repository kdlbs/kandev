package websocket

// Queue recovery actions are internal authenticated task MCP transport shims.
// Raw WebSocket clients must not dispatch this namespace.
const (
	ActionMCPGetTerminalSessionMessageQueue = "mcp.get_terminal_session_message_queue"
	ActionMCPGetChildQueueInventory         = "mcp.get_child_queue_inventory"
	ActionMCPGetChildSessionMessageQueue    = "mcp.get_child_session_message_queue"
)
