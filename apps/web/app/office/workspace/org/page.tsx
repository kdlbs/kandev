"use client";

import { useAppStore } from "@/components/state-provider";
import { useOfficeAgentsData } from "@/hooks/domains/office/use-office-data";
import { OrgChartCanvas } from "./org-chart-canvas";

export default function OrgPage() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agents = useOfficeAgentsData(workspaceId).data?.agents ?? [];

  return (
    <div className="flex flex-col h-full">
      <OrgChartCanvas agents={agents} />
    </div>
  );
}
