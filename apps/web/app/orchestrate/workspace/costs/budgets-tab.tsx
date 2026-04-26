"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "sonner";
import { listBudgets, deleteBudget } from "@/lib/api/domains/orchestrate-api";
import type { BudgetPolicy } from "@/lib/state/slices/orchestrate/types";
import { BudgetPolicyCard } from "./budget-policy-card";
import { CreateBudgetForm } from "./create-budget-form";

export function BudgetsTab({ workspaceId }: { workspaceId: string }) {
  const [policies, setPolicies] = useState<BudgetPolicy[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    listBudgets(workspaceId)
      .then((res) => setPolicies(res.budgets ?? []))
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : "Failed to load budgets");
      });
  }, [workspaceId, reloadKey]);

  const handleDelete = async (id: string) => {
    try {
      await deleteBudget(id);
      setPolicies((prev) => prev.filter((p) => p.id !== id));
      toast.success("Budget policy deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete budget policy");
    }
  };

  const handleCreated = () => {
    setShowCreate(false);
    setReloadKey((k) => k + 1);
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-sm font-semibold">Budget Policies</h2>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer"
          onClick={() => setShowCreate(!showCreate)}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          Add Policy
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
        <p className="text-sm text-muted-foreground">No budget policies configured.</p>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        {policies.map((p) => (
          <BudgetPolicyCard key={p.id} policy={p} onDelete={handleDelete} />
        ))}
      </div>
    </div>
  );
}
