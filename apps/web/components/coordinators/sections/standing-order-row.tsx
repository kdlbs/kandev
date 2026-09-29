"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";
import { formatTimestampDate } from "@/lib/coordinators/goal-form";
import { formatRelativeTime } from "@/lib/utils";

const MINUTE_MS = 60_000;

function useMinuteTick(): number {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick((value) => value + 1), MINUTE_MS);
    return () => window.clearInterval(id);
  }, []);
  return tick;
}

function useLastApplied(iso: string | null): string {
  const { t } = useTranslation();
  useMinuteTick();
  if (!iso) return t("coordinator:standingOrderNeverApplied");
  const at = Date.parse(iso);
  const when = Number.isNaN(at) || at > Date.now() ? t("common:justNow") : formatRelativeTime(iso);
  return t("coordinator:standingOrderLastApplied", { when });
}

type StandingOrderRowProps = {
  order: StandingOrder;
  canManage: boolean;
  retiring: boolean;
  onRetire: (order: StandingOrder) => void;
};

export function StandingOrderRow({ order, canManage, retiring, onRetire }: StandingOrderRowProps) {
  const { t, i18n } = useTranslation();
  const lastApplied = useLastApplied(order.last_applied_at);
  return (
    <li
      data-testid="standing-order-row"
      className="flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between"
    >
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium">
          {t("coordinator:standingOrderNumber", { number: order.number })}
        </p>
        <p className="whitespace-pre-wrap break-words text-sm">{order.text}</p>
        <p className="text-xs text-muted-foreground">
          {t("coordinator:standingOrderAdded", {
            date: formatTimestampDate(order.created_at, i18n.language),
          })}
          {" · "}
          {lastApplied}
        </p>
      </div>
      {canManage && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={retiring}
          data-testid="standing-order-retire"
          className="min-h-12 cursor-pointer sm:min-h-9"
          onClick={() => onRetire(order)}
        >
          {t("coordinator:standingOrderRetire")}
        </Button>
      )}
    </li>
  );
}
