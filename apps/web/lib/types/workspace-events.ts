export type WorkspacePayload = {
  id: string;
  name: string;
  description?: string;
  owner_id?: string;
  unit_id?: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
  default_config_agent_profile_id?: string | null;
  /**
   * Idle-suspension policy carried on workspace lifecycle events. Both keys
   * are absent from payloads sent by older backends; each consumer checks
   * key presence rather than reading the value, so it can keep its current
   * state when an older backend omits them.
   */
  acp_idle_suspension_enabled?: boolean;
  acp_idle_timeout_minutes?: number;
  /** Absent from older backends; consumers check key presence like the idle policy. */
  dynamic_midturn_failover_enabled?: boolean;
  dynamic_failed_route_auto_retry_enabled?: boolean;
  dynamic_failed_route_auto_retry_max?: number;
  /** Unanswered-question inbox settings; absent from older backends. */
  clarification_unanswered_inbox_enabled?: boolean;
  clarification_unanswered_retention_days?: number;
  created_at?: string;
  updated_at?: string;
};
