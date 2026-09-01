"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  IconCurrencyDollar,
  IconRobot,
  IconFolder,
  IconCpu,
  IconBuilding,
} from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { getCostsBreakdown } from "@/lib/api/domains/office-api";
import { MetricCard } from "../../components/metric-card";
import type { CostBreakdownItem } from "@/lib/state/slices/office/types";
import { CostBreakdownTable } from "./cost-breakdown-table";
import { formatDollars } from "@/lib/utils";
import { useTranslation } from "react-i18next";
// Module-level `t` for the error-only strings inside the fetching effect below:
// putting the hook's `t` in that dep array would re-issue the request on every
// locale switch.
import { t as staticT } from "@/lib/i18n";

type DateRange = "mtd" | "30d";

type CostOverviewData = {
  totalSubcents: number;
  byAgent: CostBreakdownItem[];
  byProject: CostBreakdownItem[];
  byModel: CostBreakdownItem[];
  byProvider: CostBreakdownItem[];
};

const EMPTY_COST_OVERVIEW: CostOverviewData = {
  totalSubcents: 0,
  byAgent: [],
  byProject: [],
  byModel: [],
  byProvider: [],
};

export function CostOverview({ workspaceId }: { workspaceId: string }) {
  const [range, setRange] = useState<DateRange>("mtd");
  const [totalSubcents, setTotalSubcents] = useState(0);
  const [byAgent, setByAgent] = useState<CostBreakdownItem[]>([]);
  const [byProject, setByProject] = useState<CostBreakdownItem[]>([]);
  const [byModel, setByModel] = useState<CostBreakdownItem[]>([]);
  const [byProvider, setByProvider] = useState<CostBreakdownItem[]>([]);
  const [loadedWorkspaceId, setLoadedWorkspaceId] = useState<string | null>(null);

  // Range changes, workspace changes, and WS-triggered refetches (below) can
  // all call fetchCosts in overlapping succession; guard against an older
  // request's response landing after a newer one and clobbering fresher data.
  const requestId = useRef(0);
  const fetchCosts = useCallback(() => {
    const current = ++requestId.current;
    // Single composed call (Stream D of office optimization). Was four
    // parallel round-trips (summary + by-agent + by-project + by-model).
    getCostsBreakdown(workspaceId)
      .then((res) => {
        if (current !== requestId.current) return;
        setLoadedWorkspaceId(workspaceId);
        setTotalSubcents(res.total_subcents ?? 0);
        setByAgent((res.by_agent ?? []) as CostBreakdownItem[]);
        setByProject((res.by_project ?? []) as CostBreakdownItem[]);
        setByModel((res.by_model ?? []) as CostBreakdownItem[]);
        setByProvider((res.by_provider ?? []) as CostBreakdownItem[]);
      })
      .catch((err) => {
        if (current !== requestId.current) return;
        toast.error(err instanceof Error ? err.message : staticT("office:failedToLoadCostData"));
      });
  }, [workspaceId]);

  useEffect(() => {
    fetchCosts();
  }, [fetchCosts, range]);

  // The WS handler bumps "costs" on a new cost event (office.cost.recorded)
  // and on a task project reassignment (office.task.updated with
  // fields: ["project_id"]) — both change the by-project breakdown for an
  // already-open tab.
  useOfficeRefetch("costs", fetchCosts);

  // Do not render a previous workspace's response while a new workspace is
  // loading. This also keeps the old values hidden when the new request
  // fails, instead of showing them under the wrong workspace.
  const displayData =
    loadedWorkspaceId === workspaceId
      ? { totalSubcents, byAgent, byProject, byModel, byProvider }
      : EMPTY_COST_OVERVIEW;

  return <CostOverviewContent range={range} onRangeChange={setRange} {...displayData} />;
}

type CostOverviewContentProps = CostOverviewData & {
  range: DateRange;
  onRangeChange: (range: DateRange) => void;
};

function CostOverviewContent({
  range,
  onRangeChange,
  totalSubcents,
  byAgent,
  byProject,
  byModel,
  byProvider,
}: CostOverviewContentProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <div className="flex gap-2">
        <Button
          variant={range === "mtd" ? "secondary" : "outline"}
          size="sm"
          className="cursor-pointer"
          onClick={() => onRangeChange("mtd")}
        >
          MTD
        </Button>
        <Button
          variant={range === "30d" ? "secondary" : "outline"}
          size="sm"
          className="cursor-pointer"
          onClick={() => onRangeChange("30d")}
        >
          {t("office:last30Days")}
        </Button>
      </div>

      <div className="grid grid-cols-2 xl:grid-cols-5 gap-2">
        <MetricCard
          icon={IconCurrencyDollar}
          label={t("office:totalSpend")}
          value={formatDollars(totalSubcents)}
        />
        <MetricCard
          icon={IconRobot}
          label={t("office:activeAgents")}
          value={String(byAgent.length)}
        />
        <MetricCard
          icon={IconFolder}
          label={t("office:projects")}
          value={String(byProject.length)}
        />
        <MetricCard icon={IconCpu} label={t("office:modelsUsed")} value={String(byModel.length)} />
        <MetricCard
          icon={IconBuilding}
          label={t("office:providers")}
          value={String(byProvider.length)}
        />
      </div>

      <div className="space-y-6">
        <CostBreakdownTable
          title={t("office:byAgent")}
          items={byAgent}
          labelPrefix={t("office:agent")}
        />
        <CostBreakdownTable
          title={t("office:byProject")}
          items={byProject}
          labelPrefix={t("office:project")}
        />
        <CostBreakdownTable
          title={t("office:byProvider")}
          items={byProvider}
          labelPrefix={t("office:provider")}
        />
        <CostBreakdownTable
          title={t("office:byModel")}
          items={byModel}
          labelPrefix={t("common:model")}
        />
      </div>
    </div>
  );
}
