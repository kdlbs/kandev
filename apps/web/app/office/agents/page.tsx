import { listAgentProfiles } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { AgentsPageClient } from "./agents-page-client";
import type { AgentProfile } from "@/lib/state/slices/office/types";

export default async function AgentsPage() {
  const workspaceId = await getActiveWorkspaceId();

  let agents: AgentProfile[] = [];
  if (workspaceId) {
    const res = await listAgentProfiles(workspaceId, { cache: "no-store" }).catch(() => ({
      agents: [],
    }));
    agents = res.agents ?? [];
  }

  return <AgentsPageClient initialAgents={agents} />;
}
