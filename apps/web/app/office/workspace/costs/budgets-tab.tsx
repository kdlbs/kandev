"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "sonner";
import { deleteBudget } from "@/lib/api/domains/office-api";
import { qk } from "@/lib/query/keys";
import { officeBudgetsQueryOptions } from "@/lib/query/query-options/office";
import { BudgetPolicyCard } from "./budget-policy-card";
import { CreateBudgetForm } from "./create-budget-form";

export function BudgetsTab({ workspaceId }: { workspaceId: string }) {
  const queryClient = useQueryClient();
  const budgetsQuery = useQuery(officeBudgetsQueryOptions(workspaceId));
  const policies = budgetsQuery.data?.budgets ?? [];
  const [showCreate, setShowCreate] = useState(false);

  const handleDelete = async (id: string) => {
    try {
      await deleteBudget(id);
      queryClient.setQueryData(qk.office.budgets(workspaceId), {
        budgets: policies.filter((p) => p.id !== id),
      });
      toast.success("Budget policy deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete budget policy");
    }
  };

  const handleCreated = () => {
    setShowCreate(false);
    void queryClient.invalidateQueries({ queryKey: qk.office.budgets(workspaceId) });
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-sm font-semibold">Budget Policies</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Set spending limits per agent or project. Agents pause when limits are exceeded.
          </p>
        </div>
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
        <p className="text-sm text-muted-foreground">
          No budget policies configured. Add one to enforce spending limits.
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
