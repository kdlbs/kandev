"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type { ClassEligibility } from "@/lib/api/domains/coordinator-automatic-api";
import type { ClassEligibilityStatus } from "@/hooks/domains/coordinator/use-class-eligibility";

const CONDITION_LABEL: Record<string, string> = {
  history_30d: "coordinator:mayDoAutomaticCondHistory",
  volume: "coordinator:mayDoAutomaticCondVolume",
  unedited_rate: "coordinator:mayDoAutomaticCondUnedited",
  no_undo: "coordinator:mayDoAutomaticCondNoUndo",
  reviewed_7d: "coordinator:mayDoAutomaticCondReviewed",
};

const TIME_FORMAT: Intl.DateTimeFormatOptions = { dateStyle: "medium", timeStyle: "short" };

function formatTime(value: string): string {
  return new Date(value).toLocaleString(undefined, TIME_FORMAT);
}

function conditionValue(name: string, value: number | string | null, t: TFunction): string {
  if (value === null || value === undefined) return t("coordinator:mayDoAutomaticValueNone");
  if (name === "unedited_rate")
    return t("coordinator:mayDoAutomaticValuePercent", { percent: value });
  if (typeof value === "string") return formatTime(value);
  return t("coordinator:mayDoAutomaticValueCount", { value });
}

export type AutomaticEligibilityProps = {
  eligibility: ClassEligibility | null;
  status: ClassEligibilityStatus;
  canManage: boolean;
  reviewOpened: boolean;
  reviewFailed: boolean;
  onMarkReviewed: () => void;
  onRetry: () => void;
};

export function AutomaticEligibility({
  eligibility,
  status,
  canManage,
  reviewOpened,
  reviewFailed,
  onMarkReviewed,
  onRetry,
}: AutomaticEligibilityProps) {
  const { t } = useTranslation();
  if (status === "error") {
    return (
      <div className="flex items-center gap-2 text-sm" data-testid="automatic-eligibility-failed">
        {t("coordinator:mayDoAutomaticLoadFailed")}
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={onRetry}>
          {t("coordinator:tryAgain")}
        </Button>
      </div>
    );
  }
  if (!eligibility) return null;
  return (
    <div className="space-y-2 rounded-md border p-3" data-testid="automatic-eligibility">
      <p className="text-sm font-medium">{t("coordinator:mayDoAutomaticEligibilityTitle")}</p>
      <ul className="space-y-1">
        {eligibility.conditions.map((condition) => (
          <li
            key={condition.name}
            className="flex flex-wrap items-center gap-2 text-sm"
            data-testid={`automatic-condition-${condition.name}`}
            data-met={condition.met}
          >
            <span className="w-16 font-medium">
              {condition.met
                ? t("coordinator:mayDoAutomaticMet")
                : t("coordinator:mayDoAutomaticNotMet")}
            </span>
            <span>{t(CONDITION_LABEL[condition.name] ?? "coordinator:mayDoAutomaticNotMet")}</span>
            <span className="text-muted-foreground">
              {conditionValue(condition.name, condition.value, t)}
            </span>
          </li>
        ))}
      </ul>
      {canManage && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            disabled={!reviewOpened}
            onClick={onMarkReviewed}
            data-testid="automatic-mark-reviewed"
          >
            {t("coordinator:mayDoAutomaticMarkReviewed")}
          </Button>
          {!reviewOpened && (
            <span className="text-xs text-muted-foreground">
              {t("coordinator:mayDoAutomaticReviewFirst")}
            </span>
          )}
        </div>
      )}
      {reviewFailed && (
        <p role="alert" className="text-xs text-destructive" data-testid="automatic-review-failed">
          {t("coordinator:mayDoAutomaticReviewFailed")}
        </p>
      )}
    </div>
  );
}

export function RaisedRecord({ eligibility }: { eligibility: ClassEligibility }) {
  const { t } = useTranslation();
  const userId = useAppStore((s) => s.auth.user?.id ?? null);
  if (eligibility.setting !== "automatic" || !eligibility.changed_at) return null;
  const name =
    userId !== null && userId === eligibility.changed_by
      ? t("coordinator:mayDoAutomaticYou")
      : t("coordinator:mayDoAutomaticAnotherManager");
  return (
    <p className="text-xs text-muted-foreground" data-testid="automatic-raised-record">
      {t("coordinator:mayDoAutomaticRaised", { name, time: formatTime(eligibility.changed_at) })}
    </p>
  );
}
