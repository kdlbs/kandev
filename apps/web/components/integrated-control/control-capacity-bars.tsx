import { useTranslation } from "react-i18next";
import type { NativeSharedCapacityWaiting } from "@/lib/types/control-native";
import type { SessionCapacityObservation } from "@/lib/types/system";
import { useControlCapacityFreshness } from "@/hooks/domains/coordinator/use-control-capacity-freshness";
import { ControlCapacitySnapshot } from "./control-capacity-snapshot";
import { capacityGradeWaiting, capacityWaitingTotal } from "./control-capacity-waiting";

const count = (value: unknown): number | null =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;

/** One usage bar per capacity grade: in use against its limit. */
export function ControlCapacityBars({ observation }: { observation?: SessionCapacityObservation }) {
  const { t } = useTranslation();
  const { known } = useControlCapacityFreshness(observation);
  if (!observation) return null;
  const grades = [
    ...new Set([
      ...Object.keys(observation.grade_limits ?? {}),
      ...Object.keys(observation.grade_in_use ?? {}),
    ]),
  ];
  return (
    <div className="grid gap-1" data-testid="control-capacity-bars">
      {grades.map((grade) => {
        const used = known ? count(observation.grade_in_use?.[grade]) : null;
        const limit = count(observation.grade_limits?.[grade]);
        const width = used !== null && limit ? Math.min(100, (used / limit) * 100) : 0;
        return (
          <div
            key={grade}
            className="grid grid-cols-[minmax(0,5rem)_1fr_auto] items-center gap-2 font-mono text-xs text-muted-foreground"
          >
            <span className="truncate" title={grade}>
              {grade}
            </span>
            <span className="h-1.5 overflow-hidden rounded bg-muted">
              <span className="block h-full bg-primary" style={{ width: `${width}%` }} />
            </span>
            <span className="tabular-nums">
              {used ?? t("common:unknown")}/{limit ?? t("common:unknown")}
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** The capacity panel folded to one line and its bars; full detail stays one click away. */
export function ControlCapacityLine({
  observation,
  lastConfirmed,
  wipWaiting,
  sharedWaiting,
}: {
  observation?: SessionCapacityObservation;
  lastConfirmed?: SessionCapacityObservation;
  wipWaiting?: number | null;
  sharedWaiting?: NativeSharedCapacityWaiting;
}) {
  const { t } = useTranslation();
  const { known } = useControlCapacityFreshness(observation);
  const unknown = t("common:unknown");
  const used = known ? count(observation?.in_use) : null;
  const waiting = known ? capacityWaitingTotal(observation) : null;
  const wip = count(wipWaiting);
  const grade = capacityGradeWaiting(observation, known, sharedWaiting);
  return (
    <section className="space-y-2 rounded-lg border p-3" data-testid="control-capacity-line">
      <p className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
        <span className="font-medium">{t("sidebar:integratedControl.capacityTitle")}</span>
        <span className="tabular-nums">
          {t("sidebar:controlSummary.capacityRunning", {
            used: used ?? unknown,
            limit: observation?.enabled ? observation.limit : unknown,
          })}
        </span>
        <span className="tabular-nums">
          {t("sidebar:controlSummary.capacityWaiting", { waiting: waiting ?? unknown })}
        </span>
        <span className="tabular-nums">
          {t("sidebar:integratedControl.waitingWip")}: {wip ?? unknown}
        </span>
        {observation?.policy === "all" && (
          <span className="tabular-nums">
            {t("sidebar:integratedControl.waitingGrade")}: {grade ?? unknown}
          </span>
        )}
      </p>
      <ControlCapacityBars observation={observation} />
      <details className="text-xs">
        <summary className="min-h-7 cursor-pointer text-muted-foreground [@media(pointer:coarse)]:min-h-11">
          {t("sidebar:controlSummary.capacityDetails")}
        </summary>
        <div className="pt-2">
          <ControlCapacitySnapshot
            observation={observation}
            lastConfirmed={lastConfirmed}
            wipWaiting={wipWaiting}
            sharedWaiting={sharedWaiting}
          />
        </div>
      </details>
    </section>
  );
}
