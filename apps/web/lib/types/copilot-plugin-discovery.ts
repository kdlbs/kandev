export type CopilotPluginAvailability = "available" | "unavailable";

export type CopilotPluginDiscoveryPlugin = {
  id: string;
  name: string;
  marketplace?: string;
  version?: string;
  native_enabled: boolean;
  availability: CopilotPluginAvailability;
  reason_code?: string;
};

export type CopilotPluginDiscoveryResponse = {
  agent_id: string;
  profile_id: string;
  provider_id: string;
  status: "ready" | "unavailable" | "unsupported";
  reason?: string;
  context_revision?: string;
  runtime_version?: string;
  /** False while native mounting still fails closed for this runtime. */
  mount_permitted?: boolean;
  plugins: CopilotPluginDiscoveryPlugin[];
};

export type CopilotPluginDiscoveryRequest = {
  profile_id?: string;
  launch_settings?: {
    env_vars: Array<{ key: string; value?: string; secret_id?: string }>;
    cli_flags: Array<{ flag: string; enabled: boolean; description?: string }>;
    command_prefix: string;
  };
  refresh?: boolean;
};
