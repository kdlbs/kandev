"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { OrgChartCanvas } from "./org-chart-canvas";

export default function OrgPage() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data: agents = [] } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  return (
    <div className="flex flex-col h-full">
      <OrgChartCanvas agents={agents} />
    </div>
  );
}
