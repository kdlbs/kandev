export type Theme = "system" | "light" | "dark";

export type KeyValue = {
  id: string;
  key: string;
  value: string;
};

export type AgentProfile = {
  id: string;
  agent: "claude-code" | "codex" | "auggie";
  name: string;
  agentDisplayName: string;
  model: string;
  autoApprove: boolean;
  temperature: number;
};
