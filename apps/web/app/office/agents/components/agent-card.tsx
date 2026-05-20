"use client";

import Link from "next/link";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import type { AgentProfile, AgentRoutePreview } from "@/lib/state/slices/office/types";
import { AgentAvatar } from "../../components/agent-avatar";
// (path from /agents/components/ → /office/components/ resolves correctly)
import { AgentStatusDot } from "./agent-status-dot";
import { AgentRoleBadge } from "./agent-role-badge";
import { BudgetGauge } from "./budget-gauge";
import { providerLabel } from "../../workspace/routing/components/provider-order-editor";

type AgentCardProps = {
  agent: AgentProfile;
};

export function AgentCard({ agent }: AgentCardProps) {
  const isPending = agent.status === "pending_approval";
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const routingEnabled = useAppStore(
    (s) => s.office.routing.byWorkspace[workspaceId ?? ""]?.enabled ?? false,
  );
  const preview = useAppStore((s) =>
    workspaceId
      ? (s.office.routing.preview.byWorkspace[workspaceId] ?? []).find(
          (p) => p.agent_id === agent.id,
        )
      : undefined,
  );
  return (
    <Link href={`/office/agents/${agent.id}`} className="cursor-pointer">
      <Card
        className={`hover:border-primary/50 transition-colors${isPending ? " opacity-70" : ""}`}
      >
        <CardContent className="flex items-start gap-3 pt-4 pb-4">
          <AgentAvatar role={agent.role} name={agent.name} size="lg" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium truncate">{agent.name}</span>
              <AgentStatusDot status={agent.status} />
              {isPending && (
                <Badge className="bg-orange-100 text-orange-700 dark:bg-orange-900/50 dark:text-orange-300 text-xs">
                  Pending Approval
                </Badge>
              )}
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
            {routingEnabled && preview && <RoutingChip preview={preview} />}
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

function RoutingChip({ preview }: { preview: AgentRoutePreview }) {
  return (
    <div className="flex items-center gap-1.5 mt-2 text-[11px]">
      {preview.primary_provider_id ? (
        <span className="font-mono text-muted-foreground">
          {providerLabel(preview.primary_provider_id)}/{preview.primary_model || "?"}
        </span>
      ) : (
        <span className="text-muted-foreground italic">no route</span>
      )}
      <RoutingStatusBadge preview={preview} />
    </div>
  );
}

function RoutingStatusBadge({ preview }: { preview: AgentRoutePreview }) {
  if (preview.degraded) {
    return (
      <Badge variant="destructive" className="text-[10px] py-0 px-1">
        Fallback
      </Badge>
    );
  }
  if (preview.missing.length > 0) {
    return (
      <Badge variant="outline" className="text-[10px] py-0 px-1">
        Blocked
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="text-[10px] py-0 px-1">
      Healthy
    </Badge>
  );
}
