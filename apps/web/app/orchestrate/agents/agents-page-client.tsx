"use client";

import { useEffect, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";

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
      <div className="flex items-center justify-between">
        <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Agents
        </h1>
        <Button size="sm" className="cursor-pointer" onClick={() => setShowCreate(true)}>
          <IconPlus className="h-4 w-4 mr-1" />
          New Agent
        </Button>
      </div>

      {agents.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-sm text-muted-foreground">No agent instances yet.</p>
          <p className="text-xs text-muted-foreground mt-1">
            Create your first agent to get started with orchestration.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-4 cursor-pointer"
            onClick={() => setShowCreate(true)}
          >
            <IconPlus className="h-4 w-4 mr-1" />
            Create Agent
          </Button>
        </div>
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
