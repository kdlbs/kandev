"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { listBudgets, deleteBudget } from "@/lib/api/domains/office-api";
import type { BudgetPolicy } from "@/lib/state/slices/office/types";
import { BudgetPolicyCard } from "./budget-policy-card";
import { CreateBudgetForm } from "./create-budget-form";
import { useTranslation } from "react-i18next";
// Module-level `t` for the error-only strings inside the fetching effect below:
// putting the hook's `t` in that dep array would re-issue the request on every
// locale switch.
import { t as staticT } from "@/lib/i18n";

export function BudgetsTab({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const [policies, setPolicies] = useState<BudgetPolicy[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    listBudgets(workspaceId)
      .then((res) => setPolicies(res.budgets ?? []))
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : staticT("office:failedToLoadBudgets"));
      });
  }, [workspaceId, reloadKey]);

  const handleDelete = async (id: string) => {
    try {
      await deleteBudget(id);
      setPolicies((prev) => prev.filter((p) => p.id !== id));
      toast.success(t("office:budgetPolicyDeleted"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToDeleteBudgetPolicy"));
    }
  };

  const handleCreated = () => {
    setShowCreate(false);
    setReloadKey((k) => k + 1);
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-sm font-semibold">{t("office:budgetPolicies")}</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("office:setSpendingLimitsPerAgentOr")}
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer"
          onClick={() => setShowCreate(!showCreate)}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          {t("office:addPolicy")}
        </Button>
      </div>

      {showCreate && (
        <CreateBudgetForm
          workspaceId={workspaceId}
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      )}

      {policies.length === 0 && !showCreate && (
        <p className="text-sm text-muted-foreground">
          {t("office:noBudgetPoliciesConfiguredAddOne")}
        </p>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        {policies.map((p) => (
          <BudgetPolicyCard key={p.id} policy={p} onDelete={handleDelete} />
        ))}
      </div>
    </div>
  );
}
