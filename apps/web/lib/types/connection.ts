/**
 * Canonical WebSocket / connection status used by both the low-level
 * `WebSocketClient` and the UI `ConnectionState` slice. Replaces the two
 * parallel enums that previously needed a rename mapping in `useWebSocket`.
 *
 * Names favour the UI vocabulary (`connected` rather than `open`,
 * `disconnected` rather than `closed`/`idle`) since most readers are the
 * status banner / chrome.
 */
export type ConnectionStatus =
  | "disconnected"
  | "connecting"
  | "connected"
  | "error"
  | "reconnecting";
