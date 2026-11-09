import { useTranslation } from "react-i18next";
import type { TaskGraphEdgeKind, TaskGraphEdgeStatus } from "@/lib/api/domains/task-graph-api";
import { TASK_GRAPH_EDGE_STYLES, TASK_GRAPH_RELATED_EDGE_STYLE } from "./task-graph-edge-styles";

const EDGE_LABEL_KEYS: Array<[TaskGraphEdgeStatus, string]> = [
  ["resolved", "task:taskGraphLegendResolved"],
  ["pending", "task:taskGraphLegendPending"],
  ["failed", "task:taskGraphLegendFailed"],
];

function LineSample({ status, kind }: { status?: TaskGraphEdgeStatus; kind?: TaskGraphEdgeKind }) {
  const style =
    kind === "related"
      ? TASK_GRAPH_RELATED_EDGE_STYLE
      : TASK_GRAPH_EDGE_STYLES[status ?? "pending"];
  return (
    <svg aria-hidden="true" width="28" height="8" className="shrink-0">
      <line
        x1="0"
        y1="4"
        x2="28"
        y2="4"
        stroke={style.stroke}
        strokeWidth={style.strokeWidth}
        strokeDasharray={style.strokeDasharray}
      />
    </svg>
  );
}

export function TaskGraphLegend({ showExternal }: { showExternal: boolean }) {
  const { t } = useTranslation();
  return (
    <ul
      data-testid="task-graph-legend"
      className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-muted-foreground"
    >
      {EDGE_LABEL_KEYS.map(([status, key]) => (
        <li key={status} className="flex items-center gap-1.5">
          <LineSample status={status} />
          {t(key)}
        </li>
      ))}
      <li className="flex items-center gap-1.5">
        <LineSample kind="related" />
        {t("task:taskGraphLegendRelated")}
      </li>
      {showExternal && (
        <li className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-3 w-5 rounded-sm border border-dashed bg-muted/40"
          />
          {t("task:taskGraphLegendExternal")}
        </li>
      )}
    </ul>
  );
}
