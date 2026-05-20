"use client";

import { useAppStore } from "@/components/state-provider";
import { OrgChartCanvas } from "./org-chart-canvas";

export default function OrgPage() {
  const agents = useAppStore((s) => s.office.agentProfiles);

  return (
    <div className="flex flex-col h-full">
      <OrgChartCanvas agents={agents} />
    </div>
  );
}
