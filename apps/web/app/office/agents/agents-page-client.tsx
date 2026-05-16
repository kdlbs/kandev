"use client";

import { useCallback, useEffect, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { listAgentProfiles } from "@/lib/api/domains/office-api";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";

type AgentsPageClientProps = {
  initialAgents: AgentProfile[];
};

export function AgentsPageClient({ initialAgents }: AgentsPageClientProps) {
  const agents = useAppStore((s) => s.office.agentProfiles);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [showCreate, setShowCreate] = useState(false);
  // Mounting these hooks fetches workspace routing config + preview once;
  // every agent card reads the resolved preview from the store.
  useWorkspaceRouting(workspaceId);
  useRoutingPreview(workspaceId);

  useEffect(() => {
    if (initialAgents.length > 0) {
      setOfficeAgentProfiles(initialAgents);
    }
  }, [initialAgents, setOfficeAgentProfiles]);

  const refetchAgents = useCallback(async () => {
    if (!workspaceId) return;
    const res = await listAgentProfiles(workspaceId).catch(() => ({
      agents: [] as AgentProfile[],
    }));
    setOfficeAgentProfiles(res.agents ?? []);
  }, [workspaceId, setOfficeAgentProfiles]);

  // Fire once on mount to recover from stale SSR hydration. The SSR fetch
  // may have raced ahead of a just-created agent's DB write; this re-hit
  // ensures the store reflects the current DB state without waiting for a
  // WS event.
  useEffect(() => {
    refetchAgents();
  }, [refetchAgents]);

  useOfficeRefetch("agents", refetchAgents);

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
            <AgentCard key={agent.id} agent={agent} />
          ))}
        </div>
      )}

      <CreateAgentDialog open={showCreate} onOpenChange={setShowCreate} />
    </div>
  );
}
