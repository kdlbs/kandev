"use client";

import Link from "next/link";
import { AgentStatusDot } from "../../agents/components/agent-status-dot";
import type { OrgTreeNode } from "./org-tree-layout";
import { CARD_W } from "./org-tree-layout";

type OrgNodeCardProps = {
  node: OrgTreeNode;
};

// Role-tinted avatar palette. Roles that don't match fall through to a
// neutral muted background — every card still reads cleanly without a
// role label.
const ROLE_PALETTE: Record<string, { bg: string; fg: string }> = {
  ceo: { bg: "bg-amber-500/15", fg: "text-amber-500" },
  cto: { bg: "bg-sky-500/15", fg: "text-sky-500" },
  cfo: { bg: "bg-emerald-500/15", fg: "text-emerald-500" },
  coo: { bg: "bg-violet-500/15", fg: "text-violet-500" },
  cmo: { bg: "bg-pink-500/15", fg: "text-pink-500" },
  engineer: { bg: "bg-blue-500/15", fg: "text-blue-500" },
  designer: { bg: "bg-fuchsia-500/15", fg: "text-fuchsia-500" },
  design: { bg: "bg-fuchsia-500/15", fg: "text-fuchsia-500" },
  pm: { bg: "bg-indigo-500/15", fg: "text-indigo-500" },
  qa: { bg: "bg-teal-500/15", fg: "text-teal-500" },
  ops: { bg: "bg-orange-500/15", fg: "text-orange-500" },
};

function paletteFor(role: string | undefined): { bg: string; fg: string } {
  if (!role) return { bg: "bg-muted", fg: "text-muted-foreground" };
  const key = role.toLowerCase().trim();
  return ROLE_PALETTE[key] ?? { bg: "bg-muted", fg: "text-muted-foreground" };
}

function initialsFor(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 0 || !parts[0]) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function OrgNodeCard({ node }: OrgNodeCardProps) {
  const { agent } = node;
  const palette = paletteFor(agent.role);

  return (
    <Link
      href={`/office/agents/${agent.id}`}
      className="absolute group rounded-xl border border-border bg-card p-3.5 shadow-sm hover:shadow-md hover:border-primary/50 hover:-translate-y-0.5 cursor-pointer transition-all"
      style={{ left: node.x, top: node.y, width: CARD_W }}
    >
      <div className="flex items-center gap-3">
        <div
          className={`relative h-10 w-10 rounded-lg ${palette.bg} flex items-center justify-center shrink-0`}
        >
          <span className={`text-sm font-semibold ${palette.fg}`}>{initialsFor(agent.name)}</span>
          <AgentStatusDot
            status={agent.status}
            className="absolute -bottom-0.5 -right-0.5 ring-2 ring-card"
          />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold truncate leading-tight">{agent.name}</p>
          {agent.role && (
            <p className="text-xs text-muted-foreground truncate capitalize mt-0.5">{agent.role}</p>
          )}
          {agent.executorPreference?.type && (
            <p className="text-[10px] uppercase tracking-wide text-muted-foreground/70 truncate mt-1">
              {agent.executorPreference.type.replace(/_/g, " ")}
            </p>
          )}
        </div>
      </div>
    </Link>
  );
}
