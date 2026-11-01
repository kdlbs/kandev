// Needs-you Inbox wire types (docs/specs/ui/system-design/needs-you-inbox-01.md
// #Data-and-contracts). Mirrors apps/backend/internal/clarification/inbox_handlers.go's
// inboxBundleView / inboxListResponse / inboxHiddenBundleView / inboxHiddenListResponse.
import type { ClarificationAnswer, ClarificationQuestion, Message } from "@/lib/types/http";

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

export type ClarificationInboxAsyncOutcome = {
  answers?: ClarificationAnswer[];
  rejected?: boolean;
  reject_reason?: string;
  text?: string;
  responder_kind?: "user" | "parent";
};

export type ClarificationInboxAsyncQuestion = {
  question_id: string;
  task_id: string;
  session_id: string;
  session_state: string;
  task_title: string;
  parent_task_id?: string;
  parent_task_title?: string;
  forwarded: boolean;
  waiting: boolean;
  status: "pending" | "answered" | "rejected" | "withdrawn" | "stale";
  context: string;
  created_at: string;
  questions: ClarificationQuestion[];
  outcome?: ClarificationInboxAsyncOutcome;
  withdraw_reason?: string;
};

export type ClarificationInboxPage = {
  bundles: ClarificationInboxBundle[];
  async_questions: ClarificationInboxAsyncQuestion[];
  count: number;
  async_count: number;
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

export type ClarificationInboxHiddenAsyncQuestion = ClarificationInboxAsyncQuestion & {
  state: ClarificationInboxSidecarState;
  snooze_until: string | null;
};

export type ClarificationInboxHiddenPage = {
  bundles: ClarificationInboxHiddenBundle[];
  async_questions: ClarificationInboxHiddenAsyncQuestion[];
  count: number;
  total: number;
};

export type ClarificationInboxSnoozeDuration = "1h" | "4h" | "24h";
