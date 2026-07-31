import type { AvailableAgent, ToolStatus } from "@/lib/types/http";

export type AgentUpdatePayload = {
  agentId: string;
  status: "idle" | "running" | "error";
  message?: string;
};

export type AgentAvailableUpdatedPayload = {
  agents: AvailableAgent[];
  tools?: ToolStatus[];
};

export type AgentInstallJobPayload = {
  job_id: string;
  agent_name: string;
  status: "queued" | "running" | "succeeded" | "failed";
  output?: string;
  error?: string;
  exit_code?: number;
  started_at: string;
  finished_at?: string;
};

export type AgentInstallOutputPayload = {
  job_id: string;
  agent_name: string;
  chunk: string;
};

export type AgentUpdateJobPayload = {
  job_id: string;
  agent_name: string;
  status: "queued" | "resolving" | "updating" | "refreshing" | "succeeded" | "failed";
  current_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentUpdateOutputPayload = {
  job_id: string;
  agent_name: string;
  chunk: string;
};
