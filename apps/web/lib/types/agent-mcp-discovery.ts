export type AgentMcpDiscoveryServer = {
  id: string;
  name: string;
  plugin_name?: string;
  source_kind: string;
  credentials_available: boolean;
};

export type AgentMcpDiscoveryResponse = {
  agent_id: string;
  provider_id: string;
  status: "ready" | "unavailable" | "unsupported";
  reason?: string;
  servers: AgentMcpDiscoveryServer[];
};
