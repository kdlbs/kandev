"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconCurrencyDollar, IconRobot, IconFolder, IconCpu } from "@tabler/icons-react";
import { toast } from "sonner";
import {
  getCostSummary,
  getCostsByAgent,
  getCostsByProject,
  getCostsByModel,
} from "@/lib/api/domains/orchestrate-api";
import { MetricCard } from "../../components/metric-card";
import type { CostBreakdownItem } from "@/lib/state/slices/orchestrate/types";
import { CostBreakdownTable } from "./cost-breakdown-table";

type DateRange = "mtd" | "30d";

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

type BreakdownResponse = { breakdown: CostBreakdownItem[] };

function loadCostData(workspaceId: string) {
  return Promise.all([
    getCostSummary(workspaceId),
    getCostsByAgent(workspaceId) as unknown as Promise<BreakdownResponse>,
    getCostsByProject(workspaceId) as unknown as Promise<BreakdownResponse>,
    getCostsByModel(workspaceId) as unknown as Promise<BreakdownResponse>,
  ]);
}

export function CostOverview({ workspaceId }: { workspaceId: string }) {
  const [range, setRange] = useState<DateRange>("mtd");
  const [totalCents, setTotalCents] = useState(0);
  const [byAgent, setByAgent] = useState<CostBreakdownItem[]>([]);
  const [byProject, setByProject] = useState<CostBreakdownItem[]>([]);
  const [byModel, setByModel] = useState<CostBreakdownItem[]>([]);

  useEffect(() => {
    loadCostData(workspaceId)
      .then(([summary, agents, projects, models]) => {
        setTotalCents(summary.total_cents ?? 0);
        setByAgent(agents.breakdown ?? []);
        setByProject(projects.breakdown ?? []);
        setByModel(models.breakdown ?? []);
      })
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : "Failed to load cost data");
      });
  }, [workspaceId, range]);

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

      <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
        <MetricCard icon={IconCurrencyDollar} label="Total Spend" value={formatCents(totalCents)} />
        <MetricCard icon={IconRobot} label="Active Agents" value={String(byAgent.length)} />
        <MetricCard icon={IconFolder} label="Projects" value={String(byProject.length)} />
        <MetricCard icon={IconCpu} label="Models Used" value={String(byModel.length)} />
      </div>

      <div className="space-y-6">
        <CostBreakdownTable title="By Agent" items={byAgent} labelPrefix="Agent" />
        <CostBreakdownTable title="By Project" items={byProject} labelPrefix="Project" />
        <CostBreakdownTable title="By Model" items={byModel} labelPrefix="Model" />
      </div>
    </div>
  );
}
