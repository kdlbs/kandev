"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { AgentInstructionsTab } from "../components/agent-instructions-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentInstructionsPage({ params }: Props) {
  const { id } = use(params);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data: agent } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
    select: (agents) => agents.find((a) => a.id === id),
  });
  if (!agent) return null;
  return <AgentInstructionsTab agent={agent} />;
}
