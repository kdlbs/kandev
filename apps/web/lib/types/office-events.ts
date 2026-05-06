// Office WS event types and payloads. Split out of backend.ts to keep
// that file under the 600-line limit. The discriminated union and
// per-event message types are re-exported from backend.ts so existing
// consumers can keep importing them from there.

import type { BackendMessage } from "./backend-message";

export type OfficeEventType =
  | "office.task.updated"
  | "office.task.created"
  | "office.task.moved"
  | "office.task.status_changed"
  | "office.comment.created"
  | "office.agent.completed"
  | "office.agent.failed"
  | "office.agent.updated"
  | "office.approval.created"
  | "office.approval.resolved"
  | "office.cost.recorded"
  | "office.run.queued"
  | "office.run.processed"
  | "office.routine.triggered"
  | "office.task.decision_recorded"
  | "office.task.review_requested"
  | "office.provider.health_changed"
  | "office.route_attempt.appended"
  | "office.routing.settings_updated";

// Generic map from backend event data — office payloads vary by event
// but all carry workspace_id / task_id / agent_profile_id at most.
export type OfficeEventPayload = {
  workspace_id?: string;
  task_id?: string;
  agent_profile_id?: string;
  [key: string]: unknown;
};

export type OfficeBackendMessageMap = {
  "office.task.updated": BackendMessage<"office.task.updated", OfficeEventPayload>;
  "office.task.created": BackendMessage<"office.task.created", OfficeEventPayload>;
  "office.task.moved": BackendMessage<"office.task.moved", OfficeEventPayload>;
  "office.task.status_changed": BackendMessage<"office.task.status_changed", OfficeEventPayload>;
  "office.comment.created": BackendMessage<"office.comment.created", OfficeEventPayload>;
  "office.agent.completed": BackendMessage<"office.agent.completed", OfficeEventPayload>;
  "office.agent.failed": BackendMessage<"office.agent.failed", OfficeEventPayload>;
  "office.agent.updated": BackendMessage<"office.agent.updated", OfficeEventPayload>;
  "office.approval.created": BackendMessage<"office.approval.created", OfficeEventPayload>;
  "office.approval.resolved": BackendMessage<"office.approval.resolved", OfficeEventPayload>;
  "office.cost.recorded": BackendMessage<"office.cost.recorded", OfficeEventPayload>;
  "office.run.queued": BackendMessage<"office.run.queued", OfficeEventPayload>;
  "office.run.processed": BackendMessage<"office.run.processed", OfficeEventPayload>;
  "office.routine.triggered": BackendMessage<"office.routine.triggered", OfficeEventPayload>;
  "office.task.decision_recorded": BackendMessage<
    "office.task.decision_recorded",
    OfficeEventPayload
  >;
  "office.task.review_requested": BackendMessage<
    "office.task.review_requested",
    OfficeEventPayload
  >;
  "office.provider.health_changed": BackendMessage<
    "office.provider.health_changed",
    OfficeEventPayload
  >;
  "office.route_attempt.appended": BackendMessage<
    "office.route_attempt.appended",
    OfficeEventPayload
  >;
  "office.routing.settings_updated": BackendMessage<
    "office.routing.settings_updated",
    OfficeEventPayload
  >;
};
