export const NOTIFICATION_EVENT_SESSION_TURN_FINISHED = "session.turn_finished";
export const NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED = "session.clarification_requested";
export const NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE = "system.update_available";
export const NOTIFICATION_EVENT_OFFICE_INBOX_ITEM = "office.inbox_item";

export const DEFAULT_NOTIFICATION_EVENTS = [NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED];

export const EVENT_LABELS: Record<string, { title: string; description: string }> = {
  [NOTIFICATION_EVENT_SESSION_TURN_FINISHED]: {
    title: "Agent turn finished",
    description: "Notify after each completed agent turn.",
  },
  [NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]: {
    title: "Agent needs an answer",
    description: "Notify when the agent explicitly asks you a question.",
  },
  [NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]: {
    title: "Kandev update available",
    description: "Notify when a newer Kandev release is available.",
  },
  [NOTIFICATION_EVENT_OFFICE_INBOX_ITEM]: {
    title: "Office inbox item",
    description: "Notify when a new inbox item needs your attention.",
  },
};
