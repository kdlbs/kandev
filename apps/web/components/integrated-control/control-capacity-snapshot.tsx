import Link from "@/components/routing/app-link";
import { controlHref } from "@/lib/navigation/integrated-control";
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { NativeSharedCapacityWaiting } from "@/lib/types/control-native";
import type { SessionCapacityObservation } from "@/lib/types/system";
import { ControlCapacityReadState } from "./control-capacity-read-state";
import { capacityGradeWaiting } from "./control-capacity-waiting";
import { useControlCapacityFreshness } from "@/hooks/domains/coordinator/use-control-capacity-freshness";

const WAIT_LABELS = {
  ceiling_total_limit: "sidebar:integratedControl.capacityWaitingTotal",
  ceiling_grade_limit: "sidebar:integratedControl.capacityWaitingGrade",
  ceiling_both_limits: "sidebar:integratedControl.capacityWaitingBoth",
  ceiling_needs_configuration: "sidebar:integratedControl.capacityWaitingConfiguration",
  ceiling_unknown_population: "sidebar:integratedControl.capacityWaitingUnknown",
} as const;

const UNKNOWN_KEY = "common:unknown";

function observedCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function countLabel(value: unknown, known: boolean, unknown: string) {
  return known && observedCount(value) ? String(value) : unknown;
}

function freshnessKey(known: boolean, stale: boolean) {
  if (!known) return "sidebar:integratedControl.capacityUnavailable";
  if (stale) return "sidebar:integratedControl.capacityStale";
  return "sidebar:integratedControl.capacityCurrent";
}

export function ControlCapacitySnapshot({
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
  const unknown = t(UNKNOWN_KEY);
  const { validTime, known, stale } = useControlCapacityFreshness(observation);
  return (
    <section className="space-y-3 rounded-lg border p-4" data-testid="control-capacity-snapshot">
      <h2 className="font-medium">{t("sidebar:integratedControl.capacityTitle")}</h2>
      <ControlCapacityReadState lastConfirmed={lastConfirmed} />
      {!observation ? (
        <p>{t("sidebar:integratedControl.capacityUnavailable")}</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {t(
              observation.policy === "all"
                ? "system:sessionCapacityAllHelp"
                : "system:sessionCapacityScope",
            )}
          </p>
          <p
            data-testid="control-capacity-freshness"
            role="status"
            className="text-xs text-muted-foreground"
          >
            {t(freshnessKey(known, stale))}
          </p>
          {validTime && (
            <p className="text-xs text-muted-foreground">
              {t("sidebar:controlRefresh.capacityResponseAt")}:{" "}
              <time dateTime={observation.observed_at}>
                {formatDateTime(observation.observed_at, {
                  year: "numeric",
                  month: "short",
                  day: "numeric",
                  hour: "numeric",
                  minute: "2-digit",
                  timeZoneName: "short",
                })}
              </time>
            </p>
          )}
          {!known && <CapacityUnknownReason validTime={validTime} />}
          <Link
            className="inline-flex min-h-7 items-center text-sm underline [@media(pointer:coarse)]:min-h-11"
            href={controlHref("activity")}
          >
            {t("sidebar:controlInspection.openSessions")}
          </Link>
          {!observation.enabled && <p>{t("sidebar:integratedControl.capacityDisabled")}</p>}
          <dl>
            <dt>{t("sidebar:integratedControl.capacityTotal")}</dt>
            <dd data-testid="control-capacity-total" className="tabular-nums">
              {countLabel(observation.in_use, known, unknown)}
              {observation.enabled && <> / {countLabel(observation.limit, true, unknown)}</>}
            </dd>
          </dl>
          {observation.policy === "all" && (
            <CapacityGrades observation={observation} known={known} />
          )}
          <CapacityWaitingSummary
            observation={observation}
            known={known}
            wipWaiting={wipWaiting}
            sharedWaiting={sharedWaiting}
          />
          {observation.policy === "all" && (
            <CapacityWaitCounts observation={observation} validTime={validTime} />
          )}
        </>
      )}
    </section>
  );
}

function CapacityGrades({
  observation,
  known,
}: {
  observation: SessionCapacityObservation;
  known: boolean;
}) {
  const { t } = useTranslation();
  const unknown = t(UNKNOWN_KEY);
  const grades = [
    ...new Set([
      ...Object.keys(observation.grade_limits ?? {}),
      ...Object.keys(observation.grade_in_use ?? {}),
    ]),
  ];
  return (
    <div>
      <h3 className="text-sm font-medium">{t("task:launchQueueGradeHeading")}</h3>
      <dl className="grid gap-1 text-sm">
        {grades.map((grade) => (
          <div
            key={grade}
            className="flex justify-between gap-3"
            data-testid={`control-capacity-grade-${grade}`}
          >
            <dt className="break-all">{grade}</dt>
            <dd className="shrink-0 tabular-nums">
              {countLabel(observation.grade_in_use?.[grade], known, unknown)} /{" "}
              {countLabel(observation.grade_limits?.[grade], true, unknown)}
            </dd>
          </div>
        ))}
      </dl>
      {grades.length === 0 && <p>{unknown}</p>}
      {observation.needs_configuration && (
        <p className="text-amber-700 dark:text-amber-300">
          {t("task:launchQueueGradeNeedsConfiguration")}
        </p>
      )}
    </div>
  );
}

function CapacityWaitCounts({
  observation,
  validTime,
}: {
  observation: SessionCapacityObservation;
  validTime: boolean;
}) {
  const { t } = useTranslation();
  const unknown = t(UNKNOWN_KEY);
  const counts = observation.waiting?.counts;
  const known =
    validTime &&
    observation.waiting?.known === true &&
    Object.keys(WAIT_LABELS).every((reason) => observedCount(counts?.[reason]));
  return (
    <div>
      <h3 className="text-sm font-medium">{t("sidebar:integratedControl.capacityWaitCounts")}</h3>
      <dl className="grid gap-1 text-sm">
        {Object.entries(WAIT_LABELS).map(([reason, key]) => (
          <div key={reason} className="flex justify-between gap-3">
            <dt>{t(key)}</dt>
            <dd className="tabular-nums" data-testid={`control-capacity-wait-${reason}`}>
              {countLabel(counts?.[reason], known, unknown)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/** The two waiting figures the control aggregate adds: grade ceiling and board WIP. */
function CapacityWaitingSummary({
  observation,
  known,
  wipWaiting,
  sharedWaiting,
}: {
  observation: SessionCapacityObservation;
  known: boolean;
  wipWaiting?: number | null;
  sharedWaiting?: NativeSharedCapacityWaiting;
}) {
  const { t } = useTranslation();
  const unknown = t(UNKNOWN_KEY);
  const grade = capacityGradeWaiting(observation, known, sharedWaiting);
  return (
    <div data-testid="control-capacity-waiting-summary">
      <h3 className="text-sm font-medium">{t("sidebar:integratedControl.waitingTitle")}</h3>
      <dl className="grid gap-1 text-sm">
        <div className="flex justify-between gap-3">
          <dt>{t("sidebar:integratedControl.waitingGrade")}</dt>
          <dd className="tabular-nums" data-testid="control-capacity-waiting-grade">
            {grade ?? unknown}
          </dd>
        </div>
        <div className="flex justify-between gap-3">
          <dt>{t("sidebar:integratedControl.waitingWip")}</dt>
          <dd className="tabular-nums" data-testid="control-capacity-waiting-wip">
            {observedCount(wipWaiting) ? wipWaiting : unknown}
          </dd>
        </div>
      </dl>
    </div>
  );
}

function CapacityUnknownReason({ validTime }: { validTime: boolean }) {
  const { t } = useTranslation();
  return (
    <p className="text-sm text-muted-foreground">
      {t(
        validTime ? "sidebar:controlInspection.noReason" : "sidebar:controlInspection.invalidTime",
      )}
    </p>
  );
}
