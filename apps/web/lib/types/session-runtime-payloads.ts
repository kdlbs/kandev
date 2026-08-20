export type AuthMethodInfoPayload = {
  id: string;
  name: string;
  description?: string;
  terminal_auth?: {
    command: string;
    args?: string[];
    label?: string;
  };
  meta?: Record<string, unknown>;
};

export type AgentCapabilitiesPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  supports_image: boolean;
  supports_audio: boolean;
  supports_embedded_context: boolean;
  auth_methods: AuthMethodInfoPayload[];
  timestamp: string;
};

export type SessionModelInfoPayload = {
  model_id: string;
  name: string;
  description?: string;
  usage_multiplier?: string;
  meta?: Record<string, unknown>;
};

export type ConfigOptionPayload = {
  type: string;
  id: string;
  name: string;
  description?: string;
  current_value: string;
  category?: string;
  options?: { value: string; name: string; description?: string }[];
};

export type SessionModelsPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  current_model_id: string;
  models: SessionModelInfoPayload[];
  config_options: ConfigOptionPayload[];
  config_options_settled?: boolean;
  config_baseline?: Record<string, string>;
  timestamp: string;
};

export type SessionModelSelectionWarningPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  warning: {
    kind: string;
    decision_id: string;
    reason: string;
    requested_model?: string;
    effective_model?: string;
    fallback_model?: string;
    agent_id: string;
    executor_type: string;
    executor_profile_id: string;
  };
  timestamp: string;
};

export type MCPAttachmentServerPayload = {
  name: string;
  source?: "kandev" | "profile";
  transport?: string;
  target?: string;
  status: "unknown" | "delivered" | "connected" | "active" | "failed" | "filtered" | "unavailable";
  reason_code?: string;
  summary?: string;
  connection_id?: string;
  tool_count?: number;
  tools_listed_at?: string;
  tools?: Array<{
    name: string;
    description?: string;
    input_schema?: unknown;
    input_schema_truncated?: boolean;
    estimated_tokens?: number;
  }>;
  tool_catalog_truncated?: boolean;
  tool_token_estimator?: string;
};

export type MCPAttachmentAttemptPayload = {
  attachment_attempt_id: string;
  started_at: string;
  updated_at?: string;
  servers?: MCPAttachmentServerPayload[];
};

export type MCPAttachmentHistoryPayload = {
  version: number;
  current: MCPAttachmentAttemptPayload;
  previous?: MCPAttachmentAttemptPayload[];
};

export type SessionMCPStatusPayload = {
  task_id: string;
  session_id: string;
  history: MCPAttachmentHistoryPayload;
  timestamp: string;
};

export type SessionInfoPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  acp_session_id?: string;
  session_title?: string;
  session_updated_at?: string;
  session_meta?: Record<string, unknown>;
  timestamp: string;
};

export type SessionPromptUsagePayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  usage: {
    input_tokens: number;
    output_tokens: number;
    cached_read_tokens?: number;
    cached_write_tokens?: number;
    total_tokens: number;
  };
  timestamp: string;
};

export type SessionTodosPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  entries: Array<{
    description: string;
    status: string;
    priority: string;
  }>;
  timestamp: string;
};
