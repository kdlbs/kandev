"use client";

import { createContext, useContext } from "react";

export type AgentProjectTaskContextValue = {
  taskId: string;
  workspaceId: string;
  projectId: string;
  tier: "coordinator" | "economy" | "frontier";
};

const AgentProjectTaskContext = createContext<AgentProjectTaskContextValue | null>(null);

export function AgentProjectTaskProvider({
  value,
  children,
}: {
  value: AgentProjectTaskContextValue | null;
  children: React.ReactNode;
}) {
  return (
    <AgentProjectTaskContext.Provider value={value}>{children}</AgentProjectTaskContext.Provider>
  );
}

export function useAgentProjectTaskContext() {
  return useContext(AgentProjectTaskContext);
}
