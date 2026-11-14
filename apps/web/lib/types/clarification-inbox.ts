// Needs-you Inbox wire types (docs/specs/ui/system-design/needs-you-inbox-01.md
// #Data-and-contracts). Mirrors apps/backend/internal/clarification/inbox_handlers.go's
// inboxBundleView / inboxListResponse / inboxHiddenBundleView / inboxHiddenListResponse.
import type { Message } from "@/lib/types/http";

export type ClarificationInboxBundle = {
  pending_id: string;
  task_id: string;
  session_id: string;
  session_state: string;
  task_title: string;
  created_at: string;
  context: string;
  messages: Message[];
};

export type ClarificationInboxPage = {
  bundles: ClarificationInboxBundle[];
  count: number;
  hidden_count: number;
  next_snooze_expiry: string | null;
  // Present exactly when the page was truncated; absent means exhausted.
  // Used only as a truncation boolean; v1 never echoes it back as a request
  // cursor.
  next_cursor?: string;
};

export type ClarificationInboxSidecarState = "dismissed" | "snoozed";

export type ClarificationInboxHiddenBundle = ClarificationInboxBundle & {
  state: ClarificationInboxSidecarState;
  snooze_until: string | null;
};

export type ClarificationInboxHiddenPage = {
  bundles: ClarificationInboxHiddenBundle[];
  count: number;
  total: number;
};

export type ClarificationInboxSnoozeDuration = "1h" | "4h" | "24h";

// Unanswered section (clarification-active-lifecycle AC .10 to .17): questions a
// newer turn superseded while still pending. The agent is not waiting on them;
// answering sends an ordinary late-answer message.
export type ClarificationInboxUnansweredBundle = ClarificationInboxBundle & {
  original_session_terminal: boolean;
  /** The task's current primary session, offered only after the asking session ended. */
  send_to_task_session_id: string | null;
};

export type ClarificationInboxUnansweredPage = {
  /** False while the workspace setting is off; the section is then not shown. */
  enabled: boolean;
  retention_days: number;
  bundles: ClarificationInboxUnansweredBundle[];
  count: number;
  next_cursor?: string;
};

// GET /api/v1/clarification-inbox/recovery-blocks: open session recovery
// blocks waiting on an operator decision. The decision stays on the task chat.
export type InboxRecoveryBlock = {
  block_id: string;
  task_id: string;
  task_title: string;
  session_id: string;
  session_state: string;
  reason: string;
  consumer_reference?: string;
  created_at: string;
  updated_at: string;
};

export type InboxRecoveryBlockPage = {
  blocks: InboxRecoveryBlock[];
  count: number;
  truncated: boolean;
};
