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
};
