"use client";

import { useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";

export default function AgentsPage() {
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const [showCreate, setShowCreate] = useState(false);

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
