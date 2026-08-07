"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  IconCurrencyDollar,
  IconRobot,
  IconFolder,
  IconCpu,
  IconBuilding,
} from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
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

export function CostOverview({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const [range, setRange] = useState<DateRange>("mtd");
  const [totalSubcents, setTotalSubcents] = useState(0);
  const [byAgent, setByAgent] = useState<CostBreakdownItem[]>([]);
  const [byProject, setByProject] = useState<CostBreakdownItem[]>([]);
  const [byModel, setByModel] = useState<CostBreakdownItem[]>([]);
  const [byProvider, setByProvider] = useState<CostBreakdownItem[]>([]);

  useEffect(() => {
    // Single composed call (Stream D of office optimization). Was four
    // parallel round-trips (summary + by-agent + by-project + by-model).
    getCostsBreakdown(workspaceId)
      .then((res) => {
        setTotalSubcents(res.total_subcents ?? 0);
        setByAgent((res.by_agent ?? []) as CostBreakdownItem[]);
        setByProject((res.by_project ?? []) as CostBreakdownItem[]);
        setByModel((res.by_model ?? []) as CostBreakdownItem[]);
        setByProvider((res.by_provider ?? []) as CostBreakdownItem[]);
      })
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : staticT("office:failedToLoadCostData"));
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
