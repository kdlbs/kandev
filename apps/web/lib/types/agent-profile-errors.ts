export type ActiveSessionInfo = {
  task_id: string;
  task_title: string;
  is_ephemeral: boolean;
};

// WatcherReference points at one issue/PR watcher row that uses the agent
// profile being deleted. Mirrors the Go shape returned from
// /api/v1/agent-profiles/:id?force=false on a 409 conflict.
export type WatcherReference = {
  id: string;
  kind: "linear" | "jira" | "github_issue" | "github_review";
  label: string;
};
