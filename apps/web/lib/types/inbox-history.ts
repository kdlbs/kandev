// Inbox History wire types (docs/specs/ui/requirements/inbox-history.md,
// docs/specs/ui/system-design/inbox-history.md). Mirrors
// apps/backend/internal/clarification/inbox_history_handlers.go's
// inboxHistoryBundleView / inboxHistoryListResponse.
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";

export type InboxHistoryReason = "superseded" | "session_ended" | "unreadable";

export type InboxHistoryBundleKind = "clarification" | "permission";

// Extends the shipped bundle shape (clarification fields reuse the same
// `context`/`messages` projection) with the exclusion reason and the turn
// identity.
export type InboxHistoryBundle = ClarificationInboxBundle & {
  kind: InboxHistoryBundleKind;
  reason: InboxHistoryReason;
  asking_turn_id: string;
  superseding_turn_id?: string;
  // Absent when the owning task's current step could not be read: the row
  // omits the label rather than guessing.
  step_starts_no_agent?: boolean;
};

export type InboxHistoryPage = {
  bundles: InboxHistoryBundle[];
  count: number;
  total: number;
  // Present exactly when the page was truncated; absent means exhausted.
  next_cursor?: string;
};

// A permission message's own metadata shape (system design "A permission
// record is a different shape"): no `question`, choices are `options[]`
// entries carrying `name`, not `label`.
export type InboxHistoryPermissionOption = {
  kind: string;
  name: string;
  option_id: string;
};

export type InboxHistoryPermissionMetadata = {
  options?: InboxHistoryPermissionOption[];
};
