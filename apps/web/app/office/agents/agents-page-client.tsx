"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";

export function AgentsPageClient() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [showCreate, setShowCreate] = useState(false);
  const { data: agents = [] } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  // Mount routing hooks to prefetch workspace routing config + preview so
  // every agent card reads resolved preview from the TQ cache.
  useWorkspaceRouting(workspaceId);
  useRoutingPreview(workspaceId);

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
