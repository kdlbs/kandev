import { Badge } from "@kandev/ui/badge";
import type { AgentRole } from "@/lib/state/slices/orchestrate/types";

const roleColors: Record<AgentRole, string> = {
  ceo: "bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300",
  worker: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
  specialist: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300",
  assistant: "bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300",
};

type AgentRoleBadgeProps = {
  role: AgentRole;
};

export function AgentRoleBadge({ role }: AgentRoleBadgeProps) {
  return <Badge className={roleColors[role]}>{role}</Badge>;
}
