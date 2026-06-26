"use client";

import { useMemo, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { useOfficeAgentsData } from "@/hooks/domains/office/use-office-data";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";

type AgentsPageClientProps = {
  initialAgents: AgentProfile[];
};

export function AgentsPageClient({ initialAgents }: AgentsPageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agentsStore = useAppStore((s) => s.office.agentProfiles);
  const agentsQuery = useOfficeAgentsData(workspaceId, initialAgents);
  const [showCreate, setShowCreate] = useState(false);
  const routing = useWorkspaceRouting(workspaceId);
  const routingPreview = useRoutingPreview(workspaceId);

  useOfficeRefetch("agents", () => void agentsQuery.refetch());

  const agents = agentsQuery.data?.agents ?? agentsStore;
  const previewsByAgentId = useMemo(
    () => new Map(routingPreview.agents.map((preview) => [preview.agent_id, preview])),
    [routingPreview.agents],
  );

  return (
    <div className="p-6 space-y-4">
      <PageHeader
        title="Agents"
        action={
          <Button size="sm" className="cursor-pointer" onClick={() => setShowCreate(true)}>
            <IconPlus className="h-4 w-4 mr-1" />
            New Agent
          </Button>
        }
      />

      {agents.length === 0 ? (
        <EmptyState
          message="No agents yet."
          description="Create a CEO agent to start orchestrating work across your projects."
          action={
            <Button
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={() => setShowCreate(true)}
            >
              <IconPlus className="h-4 w-4 mr-1" />
              Create Agent
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {agents.map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
              routingEnabled={routing.config?.enabled ?? false}
              routingPreview={previewsByAgentId.get(agent.id)}
            />
          ))}
        </div>
      )}

      <CreateAgentDialog open={showCreate} onOpenChange={setShowCreate} />
    </div>
  );
}
