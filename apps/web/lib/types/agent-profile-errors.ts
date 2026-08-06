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

export type RoutingTierReference = {
  workspace_id: string;
  provider_id: string;
  tier: "frontier" | "balanced" | "economy";
};

// AutomationReference points at one enabled automation bound to the agent
// profile being deleted. Mirrors the Go shape on the same 409. An automation is
// configuration rather than a session — nothing is running, so it never appears
// in activeSessions, but its next firing would launch against a profile that no
// longer exists.
export type AutomationReference = {
  id: string;
  name: string;
  workspace_id: string;
};
