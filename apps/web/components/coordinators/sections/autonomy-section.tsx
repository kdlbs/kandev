"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAutonomy } from "@/hooks/domains/coordinator/use-autonomy";
import {
  useAutonomySettings,
  type CeilingHint,
} from "@/hooks/domains/coordinator/use-autonomy-settings";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";
import { AutonomySpend } from "@/components/coordinators/autonomy/autonomy-spend";
import { AutonomyContainment } from "./autonomy-containment";

const HINT_KEY: Record<Exclude<CeilingHint, null>, string> = {
  invalid: "coordinator:autonomyCeilingInvalid",
  required: "coordinator:autonomyCeilingRequired",
  rejected: "coordinator:autonomyCeilingRejected",
};

function SpendLines({ value }: { value: AutonomyRead }) {
  const { t } = useTranslation();
  const { spend, last_turn: turn } = value;
  const mean =
    spend.mean_known && spend.mean_daily_subcents_7d !== null
      ? t("coordinator:spendMean", { amount: formatSubcentsUsd(spend.mean_daily_subcents_7d) })
      : t("coordinator:spendMeanUnavailable");
  let last = t("coordinator:spendNoTurns");
  if (turn) {
    last =
      turn.cost_subcents === null
        ? t("coordinator:spendLastTurnCostUnknown")
        : t("coordinator:spendLastTurn", { amount: formatSubcentsUsd(turn.cost_subcents) });
  }
  return (
    <div className="space-y-1 text-sm" data-testid="autonomy-spend-lines">
      <p>
        <AutonomySpend spend={spend} noCeilingPill />
      </p>
      <p className="text-muted-foreground" data-testid="autonomy-spend-mean">
        {mean}
      </p>
      <p className="text-muted-foreground" data-testid="autonomy-spend-last">
        {last}
      </p>
    </div>
  );
}

type Props = { workspaceId: string; coordinatorId: string; canManage: boolean };

/** The phase-3 Autonomy section: toggle, ceiling, spend and containment. */
export function AutonomySection({ workspaceId, coordinatorId, canManage }: Props) {
  const { t } = useTranslation();
  const autonomy = useAutonomy(workspaceId, coordinatorId, true);
  const settings = useAutonomySettings({
    workspaceId,
    coordinatorId,
    canManage,
    refreshAutonomy: autonomy.retry,
  });
  const { draft, status, hint, edit, retry } = settings;

  if (status === "error") {
    return (
      <div className="space-y-2" data-testid="autonomy-settings-error">
        <p className="text-sm text-destructive">{t("coordinator:loadError")}</p>
        <Button type="button" variant="outline" className="cursor-pointer" onClick={retry}>
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (!draft) return <Skeleton className="h-4 w-64" data-testid="autonomy-settings-loading" />;

  return (
    <div className="space-y-6" data-testid="autonomy-section">
      <div className="space-y-2">
        <label className="flex cursor-pointer items-center justify-between gap-3 text-sm font-medium">
          {t("coordinator:autonomyToggleLabel")}
          <Switch
            checked={draft.enabled}
            disabled={!canManage}
            onCheckedChange={(enabled) => edit({ enabled })}
            data-testid="autonomy-toggle"
          />
        </label>
        <p className="text-xs text-muted-foreground">{t("coordinator:autonomyToggleHelp")}</p>
      </div>
      <div className="space-y-2">
        <label htmlFor="autonomy-ceiling" className="text-sm font-medium">
          {t("coordinator:autonomyCeilingLabel")}
        </label>
        <Input
          id="autonomy-ceiling"
          inputMode="decimal"
          value={draft.ceiling}
          disabled={!canManage}
          aria-invalid={hint !== null}
          onChange={(event) => edit({ ceiling: event.target.value })}
          data-testid="autonomy-ceiling"
        />
        <p className="text-xs text-muted-foreground">{t("coordinator:autonomyCeilingHelp")}</p>
        {hint && (
          <p
            role="alert"
            className="text-xs text-destructive"
            data-testid="autonomy-ceiling-hint"
            data-hint={hint}
          >
            {t(HINT_KEY[hint])}
          </p>
        )}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">{t("coordinator:autonomySpendHeading")}</h3>
        {autonomy.value ? (
          <SpendLines value={autonomy.value} />
        ) : (
          <Skeleton className="h-4 w-48" data-testid="autonomy-spend-loading" />
        )}
      </div>
      <AutonomyContainment
        conditions={autonomy.value?.containment.conditions ?? null}
        error={autonomy.error}
        loading={autonomy.loading}
        onCheckAgain={autonomy.retry}
      />
    </div>
  );
}
