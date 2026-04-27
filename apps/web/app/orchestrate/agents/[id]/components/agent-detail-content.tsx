"use client";

import Link from "next/link";
import { IconArrowLeft, IconRobot } from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useAppStore } from "@/components/state-provider";
import { AgentStatusDot } from "../../components/agent-status-dot";
import { AgentRoleBadge } from "../../components/agent-role-badge";
import { BudgetGauge } from "../../components/budget-gauge";
import { AgentOverviewTab } from "./agent-overview-tab";
import { AgentSkillsTab } from "./agent-skills-tab";
import { AgentMemoryTab } from "./agent-memory-tab";
import { AgentChannelsTab } from "./agent-channels-tab";

type AgentDetailContentProps = {
  agentId: string;
};

export function AgentDetailContent({ agentId }: AgentDetailContentProps) {
  const agent = useAppStore((s) => s.orchestrate.agentInstances.find((a) => a.id === agentId));

  if (!agent) {
    return (
      <div className="p-6">
        <Link
          href="/orchestrate/agents"
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground cursor-pointer"
        >
          <IconArrowLeft className="h-4 w-4" />
          Back to agents
        </Link>
        <p className="text-muted-foreground mt-4">Agent not found.</p>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <Link
        href="/orchestrate/agents"
        className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground cursor-pointer"
      >
        <IconArrowLeft className="h-4 w-4" />
        Back to agents
      </Link>

      <Card>
        <CardContent className="flex items-center gap-4 pt-6">
          <div className="h-12 w-12 rounded-lg bg-muted flex items-center justify-center shrink-0">
            <IconRobot className="h-6 w-6 text-muted-foreground" />
          </div>
          <div>
            <h2 className="text-lg font-semibold">{agent.name}</h2>
            <div className="flex items-center gap-2 mt-1">
              <AgentRoleBadge role={agent.role} />
              <AgentStatusDot status={agent.status} />
              <span className="text-sm text-muted-foreground">{agent.status}</span>
            </div>
          </div>
          <div className="ml-auto">
            <BudgetGauge budgetCents={agent.budgetMonthlyCents} />
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" className="cursor-pointer">
            Overview
          </TabsTrigger>
          <TabsTrigger value="skills" className="cursor-pointer">
            Skills
          </TabsTrigger>
          <TabsTrigger value="runs" className="cursor-pointer">
            Runs
          </TabsTrigger>
          <TabsTrigger value="memory" className="cursor-pointer">
            Memory
          </TabsTrigger>
          <TabsTrigger value="channels" className="cursor-pointer">
            Channels
          </TabsTrigger>
        </TabsList>
        <TabsContent value="overview">
          <AgentOverviewTab agent={agent} />
        </TabsContent>
        <TabsContent value="skills">
          <AgentSkillsTab agent={agent} />
        </TabsContent>
        <TabsContent value="runs">
          <PlaceholderTab label="Runs" />
        </TabsContent>
        <TabsContent value="memory">
          <AgentMemoryTab agent={agent} />
        </TabsContent>
        <TabsContent value="channels">
          <AgentChannelsTab agent={agent} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function PlaceholderTab({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center py-12">
      <p className="text-sm text-muted-foreground">{label} -- coming soon</p>
    </div>
  );
}
