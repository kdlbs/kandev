"use client";

import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { OrgChartCanvas } from "./org-chart-canvas";

export default function OrgPage() {
  const agents = useAppStore(selectOfficeAgentProfiles);

  return (
    <div className="flex flex-col h-full">
      <OrgChartCanvas agents={agents} />
    </div>
  );
}
