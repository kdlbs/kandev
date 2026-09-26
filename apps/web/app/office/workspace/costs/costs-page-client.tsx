"use client";

import { useEffect } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useAppStore } from "@/components/state-provider";
import type { CostSummary } from "@/lib/state/slices/office/types";
import { CostOverview } from "./cost-overview";
import { BudgetsTab } from "./budgets-tab";
import { PageHeader } from "../../components/shared/page-header";
import { useTranslation } from "react-i18next";

type CostsPageClientProps = {
  initialCostSummary: CostSummary | null;
};

export function CostsPageClient({ initialCostSummary }: CostsPageClientProps) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const setCostSummary = useAppStore((s) => s.setCostSummary);

  useEffect(() => {
    if (initialCostSummary) {
      setCostSummary(initialCostSummary);
    }
  }, [initialCostSummary, setCostSummary]);

  if (!activeWorkspaceId) {
    return (
      <div className="p-6">
        <p className="text-sm text-muted-foreground">{t("office:selectAWorkspaceToViewCosts")}</p>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      <PageHeader title={t("office:costs")} />
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" className="cursor-pointer">
            {t("office:overview")}
          </TabsTrigger>
          <TabsTrigger value="budgets" className="cursor-pointer">
            {t("office:budgets")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="mt-4">
          <CostOverview workspaceId={activeWorkspaceId} />
        </TabsContent>
        <TabsContent value="budgets" className="mt-4">
          <BudgetsTab workspaceId={activeWorkspaceId} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
