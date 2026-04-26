"use client";

import { useEffect, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";

type AgentsPageClientProps = {
  initialAgents: AgentInstance[];
};

export function AgentsPageClient({ initialAgents }: AgentsPageClientProps) {
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const setAgentInstances = useAppStore((s) => s.setAgentInstances);
  const [showCreate, setShowCreate] = useState(false);

  useEffect(() => {
    if (initialAgents.length > 0) {
      setAgentInstances(initialAgents);
    }
  }, [initialAgents, setAgentInstances]);

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
          message="No agent instances yet."
          description="Create your first agent to get started with orchestration."
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
