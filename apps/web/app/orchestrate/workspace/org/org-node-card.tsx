"use client";

import Link from "next/link";
import { IconRobot } from "@tabler/icons-react";
import { AgentStatusDot } from "../../agents/components/agent-status-dot";
import type { OrgTreeNode } from "./org-tree-layout";
import { CARD_W } from "./org-tree-layout";

type OrgNodeCardProps = {
  node: OrgTreeNode;
};

export function OrgNodeCard({ node }: OrgNodeCardProps) {
  const { agent } = node;

  return (
    <Link
      href={`/orchestrate/agents/${agent.id}`}
      className="absolute border border-border rounded-lg bg-card p-3 cursor-pointer hover:border-primary transition-colors"
      style={{ left: node.x, top: node.y, width: CARD_W }}
    >
      <div className="flex items-start gap-2">
        <div className="h-8 w-8 rounded-md bg-muted flex items-center justify-center shrink-0">
          <IconRobot className="h-4 w-4 text-muted-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium truncate">{agent.name}</p>
          <p className="text-xs text-muted-foreground truncate">{agent.role}</p>
          {agent.agentProfileId && (
            <p className="text-xs text-muted-foreground truncate">
              {agent.executorPreference?.type ?? "default"}
            </p>
          )}
        </div>
      </div>
      <AgentStatusDot status={agent.status} className="absolute bottom-2 left-3" />
    </Link>
  );
}
