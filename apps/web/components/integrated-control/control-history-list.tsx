import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { OverviewEvent, OverviewEventKind } from "@/lib/types/control-overview";
import { SectionCard, SectionCardEmpty } from "./overview-section-card";
import { historyItems, repeatedMoves } from "./control-step-moves";
import { eventLabel, OverviewEventRow, StepMoveRunRow } from "./control-history-row";
import { ControlFilterChips } from "./control-filter-chips";

const EVENT_DISPLAY_LIMIT = 50;
const NO_QUIET: string[] = [];

/**
 * What happened in the last 24 hours. Each row reads as three columns: when it
 * happened, what kind of thing it was, and what it was about. The kind carries
 * the color, so a completion and a failure do not look alike. The chips count
 * each kind and filter the list; the row cap applies after that. A task's
 * consecutive step moves fold into one row, and `quietKinds` (routine records)
 * stay out of the full list until a reader asks for them.
 */
export function OverviewLast24h({
  events,
  workspaceNames,
  emptyLabel,
  quietKinds = NO_QUIET,
}: {
  events: OverviewEvent[];
  workspaceNames: Record<string, string>;
  emptyLabel?: string;
  quietKinds?: string[];
}) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<EventFilter>("all");
  const [showQuiet, setShowQuiet] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const quiet = events.filter((event) => quietKinds.includes(event.kind)).length;
  const listed = useMemo(
    () =>
      showQuiet || kind !== "all" ? events : events.filter((e) => !quietKinds.includes(e.kind)),
    [events, quietKinds, showQuiet, kind],
  );
  const counts = useMemo(() => {
    const rows = countEventKinds(events, listed.length);
    return rows.some((entry) => entry.kind === kind) ? rows : [...rows, { kind, count: 0 }];
  }, [events, listed.length, kind]);
  const items = useMemo(
    () => historyItems(kind === "all" ? listed : listed.filter((event) => event.kind === kind)),
    [listed, kind],
  );
  const repeating = useMemo(() => new Set(repeatedMoves(events).keys()), [events]);
  const visible = showAll ? items : items.slice(0, EVENT_DISPLAY_LIMIT);
  return (
    <SectionCard
      title={t("sentry:statsPeriodLast24Hours")}
      action={
        <ControlFilterChips
          chips={counts.map(({ kind: value, count }) => ({
            value,
            count,
            label: eventLabel(t, value),
          }))}
          selected={kind}
          onSelect={setKind}
          testId="overview-event-kind"
        />
      }
    >
      {visible.some((item) => item.type === "event" && item.event.kind === "pr_merged") && (
        <p
          className="border-b border-border px-4 py-2 text-xs text-muted-foreground"
          data-testid="overview-merged-pr-scope"
        >
          {t("office:overviewEventPRMergedScope")}
        </p>
      )}
      <HistoryCountBar
        shown={visible.length}
        total={items.length}
        quiet={kind === "all" ? quiet : 0}
        showQuiet={showQuiet}
        onToggleQuiet={() => setShowQuiet((value) => !value)}
      />
      {visible.length === 0 ? (
        <HistoryEmpty label={emptyLabel} />
      ) : (
        <div className="divide-y divide-border">
          {visible.map((item, index) =>
            item.type === "moves" ? (
              <StepMoveRunRow
                key={`moves:${item.taskId}:${item.end}`}
                item={item}
                workspaceNames={workspaceNames}
                repeating={repeating.has(item.taskId)}
              />
            ) : (
              <OverviewEventRow
                key={`${item.event.workspace_id ?? ""}:${item.event.id ?? `${item.event.kind}:${item.event.task_id ?? ""}:${item.event.session_id ?? ""}:${index}`}`}
                event={item.event}
                workspaceNames={workspaceNames}
              />
            ),
          )}
        </div>
      )}
      {!showAll && items.length > EVENT_DISPLAY_LIMIT && (
        <Button
          variant="link"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => setShowAll(true)}
          data-testid="overview-event-show-all"
        >
          {t("office:overviewShowAll")}
        </Button>
      )}
    </SectionCard>
  );
}

function HistoryCountBar({
  shown,
  total,
  quiet,
  showQuiet,
  onToggleQuiet,
}: {
  shown: number;
  total: number;
  quiet: number;
  showQuiet: boolean;
  onToggleQuiet: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-2 text-xs text-muted-foreground">
      <span data-testid="overview-event-visible-count">
        {t("office:overviewShowingOf", { shown, total })}
      </span>
      {quiet > 0 && (
        <button
          type="button"
          aria-pressed={showQuiet}
          onClick={onToggleQuiet}
          className="min-h-7 cursor-pointer rounded-full border px-2.5 [@media(pointer:coarse)]:min-h-11"
          data-testid="overview-event-quiet"
        >
          {t(
            showQuiet
              ? "sidebar:controlSummary.history.hideRoutine"
              : "sidebar:controlSummary.history.showRoutine",
            { routine: quiet },
          )}
        </button>
      )}
    </div>
  );
}

function HistoryEmpty({ label }: { label?: string }) {
  if (!label) return <SectionCardEmpty />;
  return <p className="px-4 py-6 text-center text-sm text-muted-foreground">{label}</p>;
}

/** The chip selection: every kind, or one kind. */
type EventFilter = OverviewEventKind | "all";

type EventKindCounts = { kind: EventFilter; count: number }[];

/** Counts each kind that actually arrived; "all" counts what the full list shows. */
function countEventKinds(events: OverviewEvent[], listed: number): EventKindCounts {
  const counts = new Map<OverviewEventKind, number>();
  for (const event of events) {
    counts.set(event.kind, (counts.get(event.kind) ?? 0) + 1);
  }
  return [
    { kind: "all", count: listed },
    ...[...counts.entries()].map(([kind, count]) => ({ kind, count })),
  ];
}
