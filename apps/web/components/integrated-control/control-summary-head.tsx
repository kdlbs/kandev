import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { formatNumber } from "@/lib/i18n/formats";
import { useIntegratedControl } from "./integrated-control-provider";
import { useControlCapacityFreshness } from "@/hooks/domains/coordinator/use-control-capacity-freshness";
import { ControlCapacityBars } from "./control-capacity-bars";
import { toneSurfaceClass, toneTextClass } from "./control-status-pill";
import type { WorkspaceFigures } from "./control-signals";
import type { ControlSignals } from "./use-control-signals";

export const ATTENTION_ANCHOR = "control-attention";

/** Only an attributed stall signal can establish a stall, not a WIP/session gap. */
export function overallStalled(signals: ControlSignals) {
  return signals.total.running === 0 && Object.values(signals.statusById).includes("stalled");
}

function hasPendingWork(signals: ControlSignals) {
  return signals.total.inProgress > 0 && signals.total.running === 0;
}

/** One sentence that says what state the whole installation is in. */
export function ControlStatusStrip({ signals }: { signals: ControlSignals }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const capacity = control?.data?.capacity;
  const stalled = signals.complete && overallStalled(signals);
  const attention = signals.attention.length;
  let tone: "error" | "delayed" | "running" | "idle" = "running";
  let text = t("sidebar:controlSummary.strip.clear");
  if (signals.loading) {
    tone = "idle";
    text = t("common:loading");
  } else if (stalled) {
    tone = "error";
    text = t("sidebar:controlSummary.strip.stalled", {
      running: signals.total.running,
      limit: capacity?.enabled ? capacity.limit : t("common:unknown"),
      inProgress: signals.total.inProgress,
      queued: signals.total.queued,
    });
  } else if (attention > 0) {
    tone = "delayed";
    text = t("sidebar:controlSummary.strip.attention", { count: attention });
  } else if (hasPendingWork(signals)) {
    tone = "idle";
    text = t("sidebar:controlSummary.strip.waiting");
  }
  return (
    <div
      role={stalled ? "alert" : "status"}
      className={cn(
        "flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border px-4 py-3 text-sm",
        toneSurfaceClass(tone),
      )}
      data-testid="control-status-strip"
      data-tone={tone}
    >
      {stalled && (
        <span className="rounded border border-current px-1.5 text-[11px] font-bold tracking-wide">
          {t("sidebar:controlSummary.state.stalled")}
        </span>
      )}
      <p className="min-w-0 flex-1">{text}</p>
      <StatusNotes signals={signals} />
    </div>
  );
}

function StatusNotes({ signals }: { signals: ControlSignals }) {
  const { t } = useTranslation();
  return (
    <>
      {!signals.loading && !signals.complete && (
        <p className="w-full text-xs">{t("sidebar:controlSummary.strip.partial")}</p>
      )}
      {!signals.loading && signals.attention.length > 0 && (
        <a className="text-sm underline underline-offset-4" href={`#${ATTENTION_ANCHOR}`}>
          {t("sidebar:controlSummary.strip.openAttention")}
        </a>
      )}
    </>
  );
}

function Kpi({
  label,
  value,
  tone,
  children,
  testId,
}: {
  label: string;
  value: ReactNode;
  tone?: "error" | "delayed";
  children?: ReactNode;
  testId?: string;
}) {
  return (
    <section className="grid content-start gap-1 rounded-lg border p-3" data-testid={testId}>
      <h3 className="text-xs text-muted-foreground">{label}</h3>
      <p className={cn("text-2xl font-semibold tabular-nums", tone && toneTextClass(tone))}>
        {value}
      </p>
      {children}
    </section>
  );
}

/** "AIWS 2 · MPM 1": the workspaces that carry a figure, largest first. */
function breakdown(
  signals: ControlSignals,
  names: Record<string, string>,
  key: keyof WorkspaceFigures,
): string {
  return Object.entries(signals.figures)
    .filter(([, figures]) => (figures?.[key] ?? 0) > 0)
    .sort(([, a], [, b]) => (b?.[key] ?? 0) - (a?.[key] ?? 0))
    .slice(0, 3)
    .map(([id, figures]) => `${names[id] ?? id} ${formatNumber(figures?.[key] ?? 0)}`)
    .join(" · ");
}

export function ControlKpis({
  signals,
  names,
}: {
  signals: ControlSignals;
  names: Record<string, string>;
}) {
  const { t } = useTranslation();
  const capacity = useIntegratedControl()?.data?.capacity;
  const { known: capacityKnown } = useControlCapacityFreshness(capacity);
  const stalled = overallStalled(signals);
  const { total } = signals;
  const note = (text: string) =>
    text ? (
      <p className="truncate text-xs text-muted-foreground" title={text}>
        {text}
      </p>
    ) : null;
  return (
    <div
      className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(10rem,1fr))]"
      data-testid="control-kpis"
    >
      <Kpi
        label={t("sidebar:controlSummary.kpi.capacity")}
        value={
          <>
            {total.running}
            <span className="text-sm font-normal text-muted-foreground">
              {" / "}
              {capacity?.enabled ? capacity.limit : t("common:unknown")}
            </span>
          </>
        }
        tone={stalled ? "error" : undefined}
        testId="control-kpi-capacity"
      >
        <ControlCapacityBars observation={capacity} />
        {capacityKnown && signals.waiting !== null && (
          // The recorded wait counts are keyed by rejection reason, not by grade.
          <p className="truncate text-xs text-muted-foreground">
            {t("sidebar:controlSummary.capacityWaiting", { waiting: signals.waiting })}
          </p>
        )}
      </Kpi>
      <Kpi
        label={t("sidebar:controlSummary.kpi.inProgress")}
        value={total.inProgress}
        tone={stalled ? "error" : undefined}
      >
        {note(t("sidebar:controlSummary.kpi.runningSessions", { value: total.running }))}
      </Kpi>
      <Kpi label={t("sidebar:controlSummary.kpi.waitingInput")} value={total.waitingInput} />
      <Kpi
        label={t("sidebar:controlSummary.kpi.queued")}
        value={total.queued}
        tone={total.queued > 0 ? "delayed" : undefined}
      >
        {note(breakdown(signals, names, "queued"))}
      </Kpi>
      <Kpi label={t("sidebar:controlSummary.kpi.completed")} value={total.completed24h}>
        {note(breakdown(signals, names, "completed24h"))}
      </Kpi>
      <Kpi label={t("sidebar:controlSummary.kpi.moves")} value={total.moves24h}>
        {note(breakdown(signals, names, "moves24h"))}
      </Kpi>
    </div>
  );
}
