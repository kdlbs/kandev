import type { BackendMessageType } from "@/lib/types/backend";

export interface RawSessionEvent {
  type: "session.event";
  protocol_version: number;
  event_type: string;
  session_id: string;
  task_id: string | null;
  sequence: number;
  event_id: string;
  payload: unknown;
}

export type CoreSessionStream = {
  wireId: string;
  lastSeenSequence: number;
  ready: boolean;
  pendingEvents: RawSessionEvent[];
  resumeToken?: string;
  poisonRecovery?: Promise<void>;
};

const ORDERED_EVENT_ACTIONS: Readonly<Record<string, BackendMessageType>> = {
  "message.added": "session.message.added",
  "message.updated": "session.message.updated",
  "message.deleted": "session.message.deleted",
  "session.turn.started": "session.turn.started",
  "session.turn.completed": "session.turn.completed",
  "session.turn.removed": "session.turn.removed",
};
const IGNORABLE_ORDERED_EVENT_TYPES: Readonly<Record<string, true>> = {
  "session.workspace_sources.updated": true,
};

export type OrderedCoreDisposition = "project" | "ignore" | "terminal" | "poison";

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value !== "";
}

function isTimestamp(value: unknown): value is string {
  return isNonEmptyString(value) && Number.isFinite(Date.parse(value));
}

// eslint-disable-next-line complexity -- Each core event shape is checked against the ordered wire contract.
function isProjectableCorePayload(
  event: RawSessionEvent,
  payload: Record<string, unknown>,
): boolean {
  if (payload.session_id !== event.session_id) return false;
  if (event.task_id !== null && payload.task_id !== event.task_id) return false;
  if (event.task_id === null && payload.task_id !== undefined && payload.task_id !== null)
    return false;
  switch (event.event_type) {
    case "message.added":
    case "message.updated":
      if (
        !isNonEmptyString(payload.message_id) ||
        (payload.author_type !== "user" && payload.author_type !== "agent") ||
        typeof payload.content !== "string" ||
        !isTimestamp(payload.created_at)
      ) {
        return false;
      }
      return event.event_type !== "message.updated" || isTimestamp(payload.updated_at);
    case "message.deleted":
      return isNonEmptyString(payload.message_id);
    case "session.turn.removed":
      return isNonEmptyString(payload.id);
    case "session.turn.started":
      return isNonEmptyString(payload.id) && isTimestamp(payload.started_at);
    case "session.turn.completed":
      return (
        isNonEmptyString(payload.id) &&
        isTimestamp(payload.started_at) &&
        isTimestamp(payload.completed_at) &&
        isTimestamp(payload.updated_at)
      );
    case "session.removed":
      return true;
    default:
      return Boolean(IGNORABLE_ORDERED_EVENT_TYPES[event.event_type]);
  }
}

function isCoreEventPayload(event: RawSessionEvent): boolean {
  if (
    !event.payload ||
    typeof event.payload !== "object" ||
    Array.isArray(event.payload) ||
    Reflect.get(event.payload, "type") !== event.event_type
  ) {
    return false;
  }
  return isProjectableCorePayload(event, event.payload as Record<string, unknown>);
}

export function orderedCoreDisposition(event: RawSessionEvent): OrderedCoreDisposition {
  if (event.protocol_version !== 1 || !isCoreEventPayload(event)) {
    return "poison";
  }
  if (orderedCoreAction(event.event_type)) return "project";
  if (event.event_type === "session.removed") return "terminal";
  if (IGNORABLE_ORDERED_EVENT_TYPES[event.event_type]) return "ignore";
  return "poison";
}

function isPositiveSequence(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

export function isRawSessionEvent(value: unknown): value is RawSessionEvent {
  if (!value || typeof value !== "object") return false;
  const event = value as Partial<RawSessionEvent>;
  const hasNullableTaskId = event.task_id === null || typeof event.task_id === "string";
  return (
    event.type === "session.event" &&
    event.protocol_version === 1 &&
    typeof event.event_type === "string" &&
    event.event_type.length > 0 &&
    typeof event.session_id === "string" &&
    event.session_id.length > 0 &&
    hasNullableTaskId &&
    isPositiveSequence(event.sequence) &&
    typeof event.event_id === "string" &&
    event.event_id.length > 0 &&
    "payload" in event
  );
}

export function orderedCoreAction(eventType: string): BackendMessageType | undefined {
  return ORDERED_EVENT_ACTIONS[eventType];
}

export function projectCoreSessionPayload(event: RawSessionEvent): Record<string, unknown> {
  const payload: Record<string, unknown> =
    event.payload !== null && typeof event.payload === "object" && !Array.isArray(event.payload)
      ? { ...event.payload }
      : {};
  if (event.event_type === "message.added" || event.event_type === "message.updated") {
    if (typeof payload.message_type === "string") {
      payload.type = payload.message_type;
    } else if (payload.type === event.event_type) {
      // Legacy test-harness publishers use the lifecycle event type in the
      // generic `type` field and omit the message subtype.
      payload.type = "message";
    }
  }
  return payload;
}
