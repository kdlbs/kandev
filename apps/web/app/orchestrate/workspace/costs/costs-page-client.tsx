"use client";

import { useEffect } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useAppStore } from "@/components/state-provider";
import type { CostSummary } from "@/lib/state/slices/orchestrate/types";
import { CostOverview } from "./cost-overview";
import { BudgetsTab } from "./budgets-tab";
import { PageHeader } from "../../components/shared/page-header";

type CostsPageClientProps = {
  initialCostSummary: CostSummary | null;
};

export function CostsPageClient({ initialCostSummary }: CostsPageClientProps) {
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
        <p className="text-sm text-muted-foreground">Select a workspace to view costs.</p>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      <PageHeader title="Costs" />
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" className="cursor-pointer">Overview</TabsTrigger>
          <TabsTrigger value="budgets" className="cursor-pointer">Budgets</TabsTrigger>
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
