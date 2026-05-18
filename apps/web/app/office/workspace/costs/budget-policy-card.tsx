"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { IconTrash } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { BudgetPolicy } from "@/lib/state/slices/office/types";
import { cn, formatDollars } from "@/lib/utils";

type Props = {
  policy: BudgetPolicy;
  spentSubcents?: number;
  onDelete?: (id: string) => void;
};

function getBarColor(pct: number): string {
  if (pct > 90) return "bg-red-500";
  if (pct > 70) return "bg-yellow-500";
  return "bg-green-500";
}

function getBudgetStatus(pct: number): { label: string; className: string } {
  if (pct >= 100) {
    return {
      label: "Exceeded",
      className: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300",
    };
  }
  if (pct >= 80) {
    return {
      label: "Warning",
      className: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
    };
  }
  return {
    label: "Healthy",
    className: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  };
}

export function BudgetPolicyCard({ policy, spentSubcents = 0, onDelete }: Props) {
  const pct =
    policy.limitSubcents > 0
      ? Math.min(100, Math.round((spentSubcents / policy.limitSubcents) * 100))
      : 0;
  const remaining = Math.max(0, policy.limitSubcents - spentSubcents);
  const status = getBudgetStatus(pct);
  const barColor = getBarColor(pct);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <div className="flex items-center gap-2">
          <CardTitle className="text-sm">
            {policy.scopeType}: {policy.scopeId}
          </CardTitle>
          <Badge className={status.className}>{status.label}</Badge>
        </div>
        {onDelete && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="cursor-pointer"
                onClick={() => onDelete(policy.id)}
              >
                <IconTrash className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Delete policy</TooltipContent>
          </Tooltip>
        )}
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <div className="flex justify-between text-sm">
            <span>Observed</span>
            <span>
              {formatDollars(spentSubcents)} ({pct}%)
            </span>
          </div>
          <div className="h-2 bg-muted rounded-full overflow-hidden">
            <div className={cn("h-full rounded-full", barColor)} style={{ width: `${pct}%` }} />
          </div>
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>Budget: {formatDollars(policy.limitSubcents)}</span>
            <span>Remaining: {formatDollars(remaining)}</span>
          </div>
          <div className="flex gap-2 text-xs text-muted-foreground mt-1">
            <span>Period: {policy.period}</span>
            <span>Alert: {policy.alertThresholdPct}%</span>
            <span>Action: {policy.actionOnExceed.replace(/_/g, " ")}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
