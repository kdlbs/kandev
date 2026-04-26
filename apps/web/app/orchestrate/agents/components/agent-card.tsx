"use client";

import Link from "next/link";
import { IconRobot } from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { AgentStatusDot } from "./agent-status-dot";
import { AgentRoleBadge } from "./agent-role-badge";
import { BudgetGauge } from "./budget-gauge";

type AgentCardProps = {
  agent: AgentInstance;
};

export function AgentCard({ agent }: AgentCardProps) {
  return (
    <Link href={`/orchestrate/agents/${agent.id}`} className="cursor-pointer">
      <Card className="hover:border-primary/50 transition-colors">
        <CardContent className="flex items-start gap-3 pt-4 pb-4">
          <div className="h-10 w-10 rounded-lg bg-muted flex items-center justify-center shrink-0">
            <IconRobot className="h-5 w-5 text-muted-foreground" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium truncate">{agent.name}</span>
              <AgentStatusDot status={agent.status} />
            </div>
            <div className="flex items-center gap-2 mt-1">
              <AgentRoleBadge role={agent.role} />
              {agent.desiredSkills && agent.desiredSkills.length > 0 && (
                <span className="text-xs text-muted-foreground">
                  {agent.desiredSkills.length} skill{agent.desiredSkills.length !== 1 ? "s" : ""}
                </span>
              )}
            </div>
            <BudgetGauge budgetCents={agent.budgetMonthlyCents} className="mt-2" />
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
