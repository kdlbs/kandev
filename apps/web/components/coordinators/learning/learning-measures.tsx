"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { useOutcomeMeasures } from "@/hooks/domains/coordinator/use-learning";
import type { OutcomeMeasure, OutcomeMeasures } from "@/lib/api/domains/coordinator-learning-api";
import {
  DEFAULT_MEASURE_DAYS,
  MEASURE_WINDOWS,
  formatDollars,
  formatPercent,
  formatWait,
  measureHasValue,
} from "@/lib/coordinator/learning";

type Translate = ReturnType<typeof useTranslation>["t"];

function measureText(key: keyof Omit<OutcomeMeasures, "days">, m: OutcomeMeasure, t: Translate) {
  if (!measureHasValue(m) || m.value === null) {
    return t(`coordinator:learningNull_${m.null_reason ?? "no_data"}`);
  }
  switch (key) {
    case "approval_without_edit":
    case "override_recurrence":
    case "agreement":
      return t("coordinator:learningMeasureRatio", {
        value: formatPercent(m.value),
        numerator: m.numerator,
        denominator: m.denominator,
      });
    case "dollars_per_merged_task":
      return t("coordinator:learningMeasureDollars", {
        amount: formatDollars(m.value),
        count: m.denominator,
      });
    default:
      return t("coordinator:learningMeasureWait", {
        ...formatWait(m.value),
        count: m.denominator,
      });
  }
}

const ROWS: { key: keyof Omit<OutcomeMeasures, "days">; label: string }[] = [
  { key: "approval_without_edit", label: "coordinator:learningMeasureApproval" },
  { key: "override_recurrence", label: "coordinator:learningMeasureRecurrence" },
  { key: "dollars_per_merged_task", label: "coordinator:learningMeasureDollarsLabel" },
  { key: "median_wait_seconds", label: "coordinator:learningMeasureWaitLabel" },
  { key: "agreement", label: "coordinator:learningMeasureAgreement" },
];

function WindowPicker({ days, onChange }: { days: number; onChange: (days: number) => void }) {
  const { t } = useTranslation();
  return (
    <div role="group" aria-label={t("coordinator:learningMeasuresWindow")} className="flex gap-1">
      {MEASURE_WINDOWS.map((d) => (
        <Button
          key={d}
          type="button"
          size="sm"
          variant={d === days ? "default" : "outline"}
          aria-pressed={d === days}
          className="min-h-11 cursor-pointer md:min-h-8"
          onClick={() => onChange(d)}
          data-testid={`learning-window-${d}`}
        >
          {t("coordinator:learningWindowDays", { count: d })}
        </Button>
      ))}
    </div>
  );
}

type Props = { workspaceId: string; coordinatorId: string };

/** The five outcome measures over a 7, 30 or 90 day window. */
export function LearningMeasures({ workspaceId, coordinatorId }: Props) {
  const { t } = useTranslation();
  const [days, setDays] = useState(DEFAULT_MEASURE_DAYS);
  const measures = useOutcomeMeasures(workspaceId, coordinatorId, days);
  return (
    <section className="space-y-2" data-testid="learning-measures">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t("coordinator:learningMeasuresTitle")}</h3>
        <WindowPicker days={days} onChange={setDays} />
      </div>
      {measures.status === "error" && (
        <div className="space-y-2" data-testid="learning-measures-error">
          <p className="text-sm text-destructive">{t("coordinator:learningMeasuresError")}</p>
          <Button
            type="button"
            variant="outline"
            className="min-h-11 cursor-pointer"
            onClick={measures.retry}
          >
            {t("coordinator:retry")}
          </Button>
        </div>
      )}
      {measures.status === "loading" && !measures.value && (
        <Skeleton className="h-4 w-64" data-testid="learning-measures-loading" />
      )}
      {measures.value && measures.status !== "error" && (
        <dl className="grid grid-cols-1 gap-x-4 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
          {ROWS.map(({ key, label }) => (
            <div key={key} className="contents" data-testid={`learning-measure-${key}`}>
              <dt className="text-muted-foreground">{t(label)}</dt>
              <dd>{measureText(key, measures.value![key], t)}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
