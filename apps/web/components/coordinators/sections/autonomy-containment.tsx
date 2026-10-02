"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import type { AutonomyCondition } from "@/lib/api/domains/coordinator-autonomy-api";
import {
  containmentFix,
  containmentLabel,
  containmentMachineToken,
} from "@/lib/coordinator/autonomy-text";

function ConditionRow({ condition }: { condition: AutonomyCondition }) {
  const { t } = useTranslation();
  const label = containmentLabel(condition.name, t);
  const fix = condition.met ? undefined : containmentFix(condition, t);
  const token = condition.met ? undefined : containmentMachineToken(condition);
  return (
    <li
      className="space-y-1 py-2"
      data-testid={`containment-${condition.name}`}
      data-met={condition.met ? "true" : "false"}
    >
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        {label ? <span>{label}</span> : <code className="text-xs">{condition.name}</code>}
        <span
          className={condition.met ? "text-muted-foreground" : "font-medium text-destructive"}
          data-testid={`containment-status-${condition.name}`}
        >
          {condition.met ? t("coordinator:containmentMet") : t("coordinator:containmentNotMet")}
        </span>
      </div>
      {fix && <p className="text-xs text-muted-foreground">{fix}</p>}
      {token && (
        <code
          className="block break-all text-xs text-muted-foreground"
          data-testid="containment-token"
        >
          {token}
        </code>
      )}
    </li>
  );
}

type Props = {
  conditions: AutonomyCondition[] | null;
  error: boolean;
  loading: boolean;
  onCheckAgain: () => void;
};

/** The four containment conditions, with one failure surface that keeps the last list. */
export function AutonomyContainment({ conditions, error, loading, onCheckAgain }: Props) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="autonomy-containment">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t("coordinator:containmentHeading")}</h3>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={loading}
          onClick={onCheckAgain}
          data-testid="containment-check-again"
        >
          {t("coordinator:containmentCheckAgain")}
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive" data-testid="autonomy-read-error">
          {t("coordinator:autonomyUnavailable")}{" "}
          <button
            type="button"
            className="cursor-pointer underline"
            onClick={onCheckAgain}
            data-testid="autonomy-read-retry"
          >
            {t("coordinator:retry")}
          </button>
        </p>
      )}
      {conditions ? (
        <ul className="divide-y" data-testid="containment-list">
          {conditions.map((condition) => (
            <ConditionRow key={condition.name} condition={condition} />
          ))}
        </ul>
      ) : (
        !error && (
          <Skeleton
            className="h-4 w-48"
            data-testid="containment-loading"
            aria-label={t("coordinator:containmentLoading")}
          />
        )
      )}
    </div>
  );
}
