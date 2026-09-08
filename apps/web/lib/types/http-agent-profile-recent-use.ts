export const AGENT_PROFILE_RECENT_USE_CONTEXTS = [
  "task_create",
  "task_session",
  "quick_chat",
  "config_chat",
] as const;

export type AgentProfileRecentUseContext = (typeof AGENT_PROFILE_RECENT_USE_CONTEXTS)[number];

export type AgentProfileRecentUseApiRecord = {
  context: AgentProfileRecentUseContext;
  profile_ids: string[];
  revision: number;
  updated_at: string;
};
