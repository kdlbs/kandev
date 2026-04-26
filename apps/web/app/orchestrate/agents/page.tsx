import { listAgentInstances } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { AgentsPageClient } from "./agents-page-client";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";

export default async function AgentsPage() {
  const workspaceId = await getActiveWorkspaceId();

  let agents: AgentInstance[] = [];
  if (workspaceId) {
    const res = await listAgentInstances(workspaceId, { cache: "no-store" }).catch(() => ({
      agents: [],
    }));
    agents = res.agents ?? [];
  }

  return <AgentsPageClient initialAgents={agents} />;
}
