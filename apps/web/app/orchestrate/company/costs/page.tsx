"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { CostOverview } from "./cost-overview";
import { BudgetsTab } from "./budgets-tab";
import { useAppStore } from "@/components/state-provider";

export default function CostsPage() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);

  if (!activeWorkspaceId) {
    return (
      <div className="p-6">
        <p className="text-sm text-muted-foreground">Select a workspace to view costs.</p>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        Costs
      </h1>
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="budgets">Budgets</TabsTrigger>
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
