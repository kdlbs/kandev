export type SlackAuthMethod = "cookie";

export interface SlackConfig {
  authMethod: SlackAuthMethod;
  commandPrefix: string;
  /** ID of a utility agent (`utility_agents.id`) the trigger invokes for each match. */
  utilityAgentId: string;
  /** Polling cadence in seconds. Bounded server-side to [5, 600]. */
  pollIntervalSeconds: number;
  slackTeamId?: string;
  slackUserId?: string;
  lastSeenTs?: string;
  hasToken: boolean;
  hasCookie: boolean;
  lastCheckedAt?: string;
  lastOk: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SetSlackConfigRequest {
  authMethod: SlackAuthMethod;
  commandPrefix?: string;
  utilityAgentId: string;
  pollIntervalSeconds?: number;
  /** Empty on update keeps the saved value. */
  token?: string;
  /** Empty on update keeps the saved value. */
  cookie?: string;
}

export interface TestSlackConnectionResult {
  ok: boolean;
  userId?: string;
  teamId?: string;
  teamName?: string;
  url?: string;
  displayName?: string;
  error?: string;
}
