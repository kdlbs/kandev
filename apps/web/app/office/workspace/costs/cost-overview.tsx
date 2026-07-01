"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@kandev/ui/button";
import {
  IconCurrencyDollar,
  IconRobot,
  IconFolder,
  IconCpu,
  IconBuilding,
} from "@tabler/icons-react";
import { officeCostBreakdownQueryOptions } from "@/lib/query/query-options/office";
import { MetricCard } from "../../components/metric-card";
import type { CostBreakdownItem } from "@/lib/state/slices/office/types";
import { CostBreakdownTable } from "./cost-breakdown-table";
import { formatDollars } from "@/lib/utils";

type DateRange = "mtd" | "30d";

export function CostOverview({ workspaceId }: { workspaceId: string }) {
  const [range, setRange] = useState<DateRange>("mtd");
  const costsQuery = useQuery(officeCostBreakdownQueryOptions(workspaceId));
  const totalSubcents = costsQuery.data?.total_subcents ?? 0;
  const byAgent = (costsQuery.data?.by_agent ?? []) as CostBreakdownItem[];
  const byProject = (costsQuery.data?.by_project ?? []) as CostBreakdownItem[];
  const byModel = (costsQuery.data?.by_model ?? []) as CostBreakdownItem[];
  const byProvider = (costsQuery.data?.by_provider ?? []) as CostBreakdownItem[];

  return (
    <div className="space-y-6">
      <div className="flex gap-2">
        <Button
          variant={range === "mtd" ? "secondary" : "outline"}
          size="sm"
          className="cursor-pointer"
          onClick={() => setRange("mtd")}
        >
          MTD
        </Button>
        <Button
          variant={range === "30d" ? "secondary" : "outline"}
          size="sm"
          className="cursor-pointer"
          onClick={() => setRange("30d")}
        >
          Last 30 days
        </Button>
      </div>

      <div className="grid grid-cols-2 xl:grid-cols-5 gap-2">
        <MetricCard
          icon={IconCurrencyDollar}
          label="Total Spend"
          value={formatDollars(totalSubcents)}
        />
        <MetricCard icon={IconRobot} label="Active Agents" value={String(byAgent.length)} />
        <MetricCard icon={IconFolder} label="Projects" value={String(byProject.length)} />
        <MetricCard icon={IconCpu} label="Models Used" value={String(byModel.length)} />
        <MetricCard icon={IconBuilding} label="Providers" value={String(byProvider.length)} />
      </div>

      <div className="space-y-6">
        <CostBreakdownTable title="By Agent" items={byAgent} labelPrefix="Agent" />
        <CostBreakdownTable title="By Project" items={byProject} labelPrefix="Project" />
        <CostBreakdownTable title="By Provider" items={byProvider} labelPrefix="Provider" />
        <CostBreakdownTable title="By Model" items={byModel} labelPrefix="Model" />
      </div>
    </div>
  );
}
