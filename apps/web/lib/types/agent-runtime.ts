export type AgentRuntimeAvailabilityStatus = "available" | "unavailable";

export type AgentRuntimeAvailabilityReason = "agentctl_exited";

export type AgentRuntimeAvailability =
  | { status: Extract<AgentRuntimeAvailabilityStatus, "available"> }
  | {
      status: Extract<AgentRuntimeAvailabilityStatus, "unavailable">;
      reason: AgentRuntimeAvailabilityReason;
      occurred_at: string;
    };
