"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { useDreamList } from "@/hooks/domains/coordinator/use-learning";
import type { DreamSummary } from "@/lib/api/domains/coordinator-learning-api";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";
import { formatDateTime } from "@/lib/i18n/formats";

type Translate = ReturnType<typeof useTranslation>["t"];

export function costText(cost: number | null, t: Translate): string {
  return cost === null
    ? t("coordinator:learningCostUnknown")
    : t("coordinator:learningCost", { amount: formatSubcentsUsd(cost) });
}

function ReportRow({ dream, onOpen }: { dream: DreamSummary; onOpen: (id: string) => void }) {
  const { t } = useTranslation();
  return (
    <li
      className="flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid={`learning-report-${dream.id}`}
    >
      <p className="text-sm">
        <span className="font-medium">{formatDateTime(dream.started_at)}</span>{" "}
        <span data-testid="learning-report-status">
          {t(`coordinator:learningStatus_${dream.status}`)}
        </span>
        {". "}
        {t("coordinator:learningTurns", { count: dream.turn_count })}
        {", "}
        {t("coordinator:learningItems", { count: dream.item_count })}
        {", "}
        {costText(dream.cost_subcents, t)}
      </p>
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer sm:min-h-8"
        onClick={() => onOpen(dream.id)}
        data-testid={`learning-report-open-${dream.id}`}
      >
        {t("coordinator:learningOpen")}
      </Button>
    </li>
  );
}

type Props = { workspaceId: string; coordinatorId: string; onOpen: (id: string) => void };

/** The newest-first reports list, 20 to a page, older pages on demand. */
export function LearningReports({ workspaceId, coordinatorId, onOpen }: Props) {
  const { t } = useTranslation();
  const list = useDreamList(workspaceId, coordinatorId);
  let body;
  if (list.status === "error" && list.dreams.length === 0) {
    body = (
      <div className="space-y-2" data-testid="learning-reports-error">
        <p className="text-sm text-destructive">{t("coordinator:learningReportsError")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer"
          onClick={list.retry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  } else if (list.status === "loading" && list.dreams.length === 0) {
    body = <Skeleton className="h-4 w-64" data-testid="learning-reports-loading" />;
  } else if (list.dreams.length === 0) {
    body = (
      <p className="text-sm text-muted-foreground" data-testid="learning-reports-empty">
        {t("coordinator:learningReportsEmpty")}
      </p>
    );
  } else {
    body = (
      <ul className="space-y-2">
        {list.dreams.map((d) => (
          <ReportRow key={d.id} dream={d} onOpen={onOpen} />
        ))}
      </ul>
    );
  }
  return (
    <section className="space-y-2" data-testid="learning-reports">
      <h3 className="text-sm font-medium">{t("coordinator:learningReportsTitle")}</h3>
      {body}
      {list.moreFailed && (
        <p className="text-sm text-destructive">{t("coordinator:learningReportsError")}</p>
      )}
      {list.hasMore && (
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer"
          onClick={() => void list.loadMore()}
          data-testid="learning-reports-more"
        >
          {t("coordinator:learningLoadMore")}
        </Button>
      )}
    </section>
  );
}
