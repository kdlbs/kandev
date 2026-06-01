// Base WS envelope shape used by all backend messages. Split out of
// backend.ts so sibling event-type files (e.g. office-events.ts) can
// build their per-event message maps without re-importing backend.ts
// and forming a cycle.

export type BackendMessage<T extends string, P> = {
  id?: string;
  type: "request" | "response" | "notification" | "error";
  action: T;
  payload: P;
  timestamp?: string;
  // Phase 1 WS event accounting (see lib/ws/ws-account.ts). Optional so
  // backward-compat messages without them don't break the type.
  seq?: number;
  connection_id?: string;
};
