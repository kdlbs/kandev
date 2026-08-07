"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { IconTrash } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { BudgetPolicy } from "@/lib/state/slices/office/types";
import { cn, formatDollars } from "@/lib/utils";
import { useTranslation } from "react-i18next";
// Module-level `t`, resolved at call time: `getBudgetStatus` is a plain helper
// called from the card's render, so there is no hook to bind. The card
// re-renders on `languageChanged` through its own `useTranslation()`.
import { t } from "@/lib/i18n";

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
      label: t("office:exceeded"),
      className: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300",
    };
  }
  if (pct >= 80) {
    return {
      label: t("office:warning"),
      className: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
    };
  }
  return {
    label: t("office:healthy"),
    className: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  };
}

export function BudgetPolicyCard({ policy, spentSubcents = 0, onDelete }: Props) {
  const { t } = useTranslation();
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
            <TooltipContent>{t("office:deletePolicy")}</TooltipContent>
          </Tooltip>
        )}
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <div className="flex justify-between text-sm">
            <span>{t("office:observed")}</span>
            <span>
              {formatDollars(spentSubcents)} ({pct}%)
            </span>
          </div>
          <div className="h-2 bg-muted rounded-full overflow-hidden">
            <div className={cn("h-full rounded-full", barColor)} style={{ width: `${pct}%` }} />
          </div>
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>{t("office:budgetAmount", { amount: formatDollars(policy.limitSubcents) })}</span>
            <span>{t("office:remainingAmount", { amount: formatDollars(remaining) })}</span>
          </div>
          <div className="flex gap-2 text-xs text-muted-foreground mt-1">
            {/* `period` and `actionOnExceed` are wire enums; only the field
                labels are copy, so the values are interpolated as-is. */}
            <span>{t("office:periodValue", { period: policy.period })}</span>
            <span>{t("office:alertPercent", { percent: policy.alertThresholdPct })}</span>
            <span>
              {t("office:actionValue", { action: policy.actionOnExceed.replace(/_/g, " ") })}
            </span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
