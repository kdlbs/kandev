// AC-UI-INBOX-FAILED-001.9a: `manual` is the only origin treated as a
// person and renders no marker; the five enumerated non-person values (mirrors
// apps/backend/internal/task/models/models.go's TaskOrigin* constants) each
// render their own distinct marker key, and anything else non-empty renders
// the generic marker rather than nothing, the raw value, or a broken row.
const ORIGIN_MARKER_KEYS: Record<string, string> = {
  agent_created: "failedInbox:originAgentCreated",
  routine: "failedInbox:originRoutine",
  onboarding: "failedInbox:originOnboarding",
  automation_run: "failedInbox:originAutomationRun",
  automation_task: "failedInbox:originAutomationTask",
};

export function failedInboxOriginMarkerKey(origin: string | undefined): string | null {
  if (!origin || origin === "manual") return null;
  return ORIGIN_MARKER_KEYS[origin] ?? "failedInbox:originGeneric";
}

// AC-UI-INBOX-FAILED-001.19: applies to the value the server returns, after
// its own truncation, so a reason whose first 512 code points are all
// whitespace still takes the fallback.
export function hasResolvableFailedInboxReason(reason: string): boolean {
  return reason.trim().length > 0;
}
