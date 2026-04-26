"use client";

import { usePathname } from "next/navigation";

const PAGE_TITLES: Record<string, string> = {
  "/orchestrate": "DASHBOARD",
  "/orchestrate/inbox": "INBOX",
  "/orchestrate/issues": "TASKS",
  "/orchestrate/routines": "ROUTINES",
  "/orchestrate/projects": "PROJECTS",
  "/orchestrate/agents": "AGENTS",
  "/orchestrate/workspace/org": "ORG CHART",
  "/orchestrate/workspace/skills": "SKILLS",
  "/orchestrate/workspace/costs": "COSTS",
  "/orchestrate/workspace/activity": "ACTIVITY",
  "/orchestrate/workspace/settings": "SETTINGS",
  "/orchestrate/workspace/settings/export": "SETTINGS > EXPORT",
};

export function OrchestrateTopbar() {
  const pathname = usePathname();

  // Match exact or find the closest parent path
  const title =
    PAGE_TITLES[pathname] ??
    Object.entries(PAGE_TITLES).find(([path]) => pathname.startsWith(path + "/"))?.[1] ??
    "ORCHESTRATE";

  return (
    <div className="flex items-center px-6 h-12 border-b border-border bg-background shrink-0">
      <h1 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {title}
      </h1>
    </div>
  );
}
