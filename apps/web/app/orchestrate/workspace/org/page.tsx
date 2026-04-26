"use client";

import { useAppStore } from "@/components/state-provider";
import { OrgChartCanvas } from "./org-chart-canvas";

export default function OrgPage() {
  const agents = useAppStore((s) => s.orchestrate.agentInstances);

  return (
    <div className="flex flex-col h-full">
      <div className="px-6 pt-6 pb-4">
        <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Org
        </h1>
      </div>
      <OrgChartCanvas agents={agents} />
    </div>
  );
}
