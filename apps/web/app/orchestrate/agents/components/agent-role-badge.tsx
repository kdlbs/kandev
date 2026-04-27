"use client";

import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import type { AgentRole } from "@/lib/state/slices/orchestrate/types";

const FALLBACK_COLORS: Record<AgentRole, string> = {
  ceo: "bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300",
  worker: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
  specialist: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300",
  assistant: "bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300",
};

type AgentRoleBadgeProps = {
  role: AgentRole;
};

export function AgentRoleBadge({ role }: AgentRoleBadgeProps) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const metaRole = meta?.roles.find((r) => r.id === role);
  const colorClass = metaRole?.color ?? FALLBACK_COLORS[role] ?? "";
  const label = metaRole?.label ?? role;
  return <Badge className={colorClass}>{label}</Badge>;
}
