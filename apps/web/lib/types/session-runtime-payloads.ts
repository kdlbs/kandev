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
  current_value: string;
  category?: string;
  options?: { value: string; name: string }[];
};

export type SessionModelsPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  current_model_id: string;
  models: SessionModelInfoPayload[];
  config_options: ConfigOptionPayload[];
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
