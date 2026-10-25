// Inbox Failed tab wire types
// (docs/specs/ui/system-design/inbox-failed-bucket-01.md#Data-and-contracts).
// Mirrors apps/backend/internal/failedinbox/handlers.go's
// failedInboxRow / failedInboxListResponse.

export type FailedInboxRow = {
  task_id: string;
  title: string;
  workspace_id: string;
  origin: string;
  // Omitted (never null, never a zero instant) when unresolvable -- how the
  // client tells an unresolved failure instant from a real one.
  failure_instant?: string;
  reason: string;
};

export type FailedInboxPage = {
  rows: FailedInboxRow[];
  count: number;
  truncated: boolean;
};
